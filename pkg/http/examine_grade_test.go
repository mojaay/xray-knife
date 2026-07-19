package http

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lilendian0x00/xray-knife/v10/pkg/core/protocol"
)

// --- Minimal stub core so ExamineConfig's real grading loop can run against a
// local httptest server, with no xray/sing-box instance or network proxy. ---

type stubInstance struct{}

func (stubInstance) Start() error { return nil }
func (stubInstance) Close() error { return nil }

type stubProtocol struct{}

func (stubProtocol) Parse() error       { return nil }
func (stubProtocol) DetailsStr() string { return "" }
func (stubProtocol) GetLink() string    { return "stub://config" }
func (stubProtocol) ConvertToGeneralConfig() protocol.GeneralConfig {
	return protocol.GeneralConfig{Protocol: "stub", Address: "127.0.0.1", Port: "0", TLS: "none"}
}

type stubCore struct{ client *http.Client }

func (stubCore) Name() string                                     { return "stub" }
func (stubCore) CreateProtocol(string) (protocol.Protocol, error) { return stubProtocol{}, nil }
func (stubCore) SetInbound(protocol.Protocol) error               { return nil }
func (stubCore) MakeInstance(context.Context, protocol.Protocol) (protocol.Instance, error) {
	return stubInstance{}, nil
}
func (c stubCore) MakeHttpClient(context.Context, protocol.Protocol, time.Duration) (*http.Client, protocol.Instance, error) {
	return c.client, stubInstance{}, nil
}

// gradeTestServer returns controllable status codes keyed by path.
func gradeTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			w.WriteHeader(http.StatusOK) // 200
		case "/generate_204":
			w.WriteHeader(http.StatusNoContent) // 204
		case "/portal/generate_204":
			// Captive portal: answers 200 to a generate_204-style probe.
			w.WriteHeader(http.StatusOK)
		case "/forbidden":
			w.WriteHeader(http.StatusForbidden) // 403
		case "/slow":
			time.Sleep(40 * time.Millisecond) // reliably exceeds a small MaxDelay
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newGradeExaminer(client *http.Client, panel []EndpointCheck, threshold float64) *Examiner {
	return &Examiner{
		Core:             stubCore{client: client},
		MaxDelay:         5000,
		Timeout:          5000,
		SuccessThreshold: threshold,
		TestEndpoints:    panel,
		Logger:           log.New(io.Discard, "", 0),
	}
}

func TestExamineConfig_Grading(t *testing.T) {
	srv := gradeTestServer(t)
	client := srv.Client()

	tests := []struct {
		name        string
		panel       []EndpointCheck
		threshold   float64
		wantStatus  string
		wantSuccess int
		wantTotal   int
	}{
		{
			name: "all endpoints pass -> passed",
			panel: []EndpointCheck{
				{URL: srv.URL + "/ok"},
				{URL: srv.URL + "/generate_204"}, // expect 204 inferred
			},
			threshold:   1.0,
			wantStatus:  "passed",
			wantSuccess: 2,
			wantTotal:   2,
		},
		{
			name: "one of two fails at 1.0 -> semi-passed",
			panel: []EndpointCheck{
				{URL: srv.URL + "/ok"},
				{URL: srv.URL + "/forbidden"},
			},
			threshold:   1.0,
			wantStatus:  "semi-passed",
			wantSuccess: 1,
			wantTotal:   2,
		},
		{
			name: "one of two passes threshold 0.5 -> passed",
			panel: []EndpointCheck{
				{URL: srv.URL + "/ok"},
				{URL: srv.URL + "/forbidden"},
			},
			threshold:   0.5,
			wantStatus:  "passed",
			wantSuccess: 1,
			wantTotal:   2,
		},
		{
			name: "all endpoints fail -> failed",
			panel: []EndpointCheck{
				{URL: srv.URL + "/forbidden"},
				{URL: srv.URL + "/portal/generate_204"}, // 200 to a 204 probe = captive portal
			},
			threshold:   1.0,
			wantStatus:  "failed",
			wantSuccess: 0,
			wantTotal:   2,
		},
		{
			name: "captive portal rejected: 200 for generate_204 -> failed",
			panel: []EndpointCheck{
				{URL: srv.URL + "/portal/generate_204"},
			},
			threshold:   1.0,
			wantStatus:  "failed",
			wantSuccess: 0,
			wantTotal:   1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newGradeExaminer(client, tc.panel, tc.threshold)
			r, _ := e.ExamineConfig(context.Background(), "stub://config")
			if r.Status != tc.wantStatus {
				t.Errorf("Status = %q, want %q (reason: %q)", r.Status, tc.wantStatus, r.Reason)
			}
			if r.SuccessCount != tc.wantSuccess {
				t.Errorf("SuccessCount = %d, want %d", r.SuccessCount, tc.wantSuccess)
			}
			if r.TotalCount != tc.wantTotal {
				t.Errorf("TotalCount = %d, want %d", r.TotalCount, tc.wantTotal)
			}
		})
	}
}

func TestExamineConfig_PerEndpointVisibility(t *testing.T) {
	srv := gradeTestServer(t)
	client := srv.Client()

	panel := []EndpointCheck{
		{URL: srv.URL + "/ok"},                  // 200 -> ok
		{URL: srv.URL + "/portal/generate_204"}, // 200 to 204 probe -> bad-status
		{URL: "http://127.0.0.1:1/unreachable"}, // connection refused -> error
	}
	e := newGradeExaminer(client, panel, 1.0)
	r, _ := e.ExamineConfig(context.Background(), "stub://config")

	if len(r.EndpointResults) != 3 {
		t.Fatalf("EndpointResults len = %d, want 3", len(r.EndpointResults))
	}
	want := []struct {
		outcome string
		hasCode bool
	}{
		{"ok", true},
		{"bad-status", true},
		{"error", false},
	}
	for i, w := range want {
		er := r.EndpointResults[i]
		if er.Outcome != w.outcome {
			t.Errorf("endpoint %d (%s): outcome = %q, want %q", i, er.Label, er.Outcome, w.outcome)
		}
		if er.Label == "" {
			t.Errorf("endpoint %d: empty label", i)
		}
		if w.outcome == "ok" && er.Delay < 0 {
			t.Errorf("endpoint %d: ok but Delay=%d", i, er.Delay)
		}
		if w.outcome == "error" && er.Delay != -1 {
			t.Errorf("endpoint %d: error but Delay=%d (want -1)", i, er.Delay)
		}
		if w.outcome != "ok" && er.Reason == "" {
			t.Errorf("endpoint %d: failed outcome with empty reason", i)
		}
	}

	// The compact summary must mention every endpoint label.
	for _, er := range r.EndpointResults {
		if !strings.Contains(r.EndpointSummary, er.Label) {
			t.Errorf("EndpointSummary %q missing label %q", r.EndpointSummary, er.Label)
		}
	}
}

func TestExamineConfig_SlowEndpointOutcome(t *testing.T) {
	srv := gradeTestServer(t)
	client := srv.Client()

	// The /slow handler sleeps ~40ms; a 5ms MaxDelay guarantees a "slow" verdict.
	e := newGradeExaminer(client, []EndpointCheck{{URL: srv.URL + "/slow"}}, 1.0)
	e.MaxDelay = 5
	r, _ := e.ExamineConfig(context.Background(), "stub://config")

	if len(r.EndpointResults) != 1 || r.EndpointResults[0].Outcome != "slow" {
		t.Fatalf("expected a single 'slow' endpoint, got %+v", r.EndpointResults)
	}
	// Single slow endpoint preserves the legacy "timeout" status.
	if r.Status != "timeout" {
		t.Errorf("Status = %q, want timeout", r.Status)
	}
}

// TestExamineConfig_SingleEndpointStatusGate is the core fix: a single endpoint
// returning a non-2xx status must now fail instead of passing on delay alone.
func TestExamineConfig_SingleEndpointStatusGate(t *testing.T) {
	srv := gradeTestServer(t)
	client := srv.Client()

	// Legacy single-endpoint mode (no panel): 403 must fail.
	eBad := newGradeExaminer(client, nil, 1.0)
	eBad.TestEndpoint = srv.URL + "/forbidden"
	rBad, errBad := eBad.ExamineConfig(context.Background(), "stub://config")
	if rBad.Status != "failed" {
		t.Errorf("403 single endpoint: Status = %q, want failed", rBad.Status)
	}
	if errBad == nil {
		t.Errorf("403 single endpoint: expected non-nil error")
	}

	// A 2xx single endpoint still passes.
	eOK := newGradeExaminer(client, nil, 1.0)
	eOK.TestEndpoint = srv.URL + "/ok"
	rOK, errOK := eOK.ExamineConfig(context.Background(), "stub://config")
	if rOK.Status != "passed" {
		t.Errorf("200 single endpoint: Status = %q, want passed", rOK.Status)
	}
	if errOK != nil {
		t.Errorf("200 single endpoint: unexpected error %v", errOK)
	}
}
