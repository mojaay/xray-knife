package http

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// useTestSpeedtestEndpoint points the package-level speed test target at srv
// for the duration of the test.
func useTestSpeedtestEndpoint(t *testing.T, srv *httptest.Server) {
	t.Helper()
	prev := speedtest
	t.Cleanup(func() { speedtest = prev })
	speedtest = &SpeedTester{
		SNI:              strings.TrimPrefix(srv.URL, "https://"),
		DownloadEndpoint: "/__down",
		UploadEndpoint:   "/__up",
		DebugEndpoint:    "/cdn-cgi/trace",
	}
}

// speedtestServer serves Cloudflare-shaped /__down and /__up endpoints that
// each take roughly `work` to complete, so a transfer can be made to outlast a
// latency-sized client timeout. uploaded records the bytes /__up received.
func speedtestServer(t *testing.T, work time.Duration, status int, uploaded *atomic.Int64) *httptest.Server {
	t.Helper()
	const chunks = 5
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/__down":
			if status != http.StatusOK {
				w.WriteHeader(status)
				return
			}
			// Headers first so the measured window is the body transfer, then
			// dribble the payload out over `work`.
			w.WriteHeader(http.StatusOK)
			w.(http.Flusher).Flush()
			total := 100_000
			for i := 0; i < chunks; i++ {
				time.Sleep(work / chunks)
				_, _ = w.Write(make([]byte, total/chunks))
				w.(http.Flusher).Flush()
			}
		case "/__up":
			n, _ := io.Copy(io.Discard, r.Body)
			if uploaded != nil {
				uploaded.Store(n)
			}
			time.Sleep(work)
			w.WriteHeader(status)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// latencyBudgetClient mimics the client ExamineConfig builds: a transport that
// can reach the server, capped by a Timeout sized for a latency probe.
func latencyBudgetClient(srv *httptest.Server, timeout time.Duration) *http.Client {
	c := srv.Client()
	return &http.Client{Transport: c.Transport, Timeout: timeout}
}

// A speed test must not inherit the latency probe's client timeout.
func TestRunSpeedtestOutlivesClientTimeout(t *testing.T) {
	var uploaded atomic.Int64
	srv := speedtestServer(t, 250*time.Millisecond, http.StatusOK, &uploaded)
	useTestSpeedtestEndpoint(t, srv)

	e := &Examiner{
		DoSpeedtest:       true,
		SpeedtestKbAmount: 100,
		SpeedtestTimeout:  10,
	}
	r := Result{Status: "passed"}
	e.runSpeedtest(context.Background(), latencyBudgetClient(srv, 50*time.Millisecond), &r)

	if r.Reason != "" {
		t.Fatalf("expected no failure reason, got %q", r.Reason)
	}
	if r.DownloadSpeed <= 0 {
		t.Errorf("DownloadSpeed = %v, want > 0", r.DownloadSpeed)
	}
	if r.UploadSpeed <= 0 {
		t.Errorf("UploadSpeed = %v, want > 0", r.UploadSpeed)
	}
	if got, want := uploaded.Load(), int64(100*1000); got != want {
		t.Errorf("server received %d upload bytes, want %d", got, want)
	}
}

// A failed direction must be reported, not left as an ambiguous 0 Mbps.
func TestRunSpeedtestReportsFailure(t *testing.T) {
	srv := speedtestServer(t, 0, http.StatusInternalServerError, nil)
	useTestSpeedtestEndpoint(t, srv)

	e := &Examiner{DoSpeedtest: true, SpeedtestKbAmount: 10, SpeedtestTimeout: 10}
	r := Result{Status: "passed"}
	e.runSpeedtest(context.Background(), latencyBudgetClient(srv, 5*time.Second), &r)

	for _, want := range []string{"speedtest_download_failed", "speedtest_upload_failed"} {
		if !strings.Contains(r.Reason, want) {
			t.Errorf("Reason = %q, want it to contain %q", r.Reason, want)
		}
	}
	if r.DownloadSpeed != 0 || r.UploadSpeed != 0 {
		t.Errorf("speeds = %v/%v, want 0/0 on failure", r.DownloadSpeed, r.UploadSpeed)
	}
	// A speed test failure says nothing about whether the config works.
	if r.Status != "passed" {
		t.Errorf("Status = %q, want it left as %q", r.Status, "passed")
	}
}

// The per-direction budget must bound the transfer, but running out of budget
// is not a failure: the bytes that did move are measured over the window.
// Otherwise every link slower than amount/budget reports 0 Mbps (issue #69).
func TestRunSpeedtestHonorsSpeedtestTimeout(t *testing.T) {
	srv := speedtestServer(t, 3*time.Second, http.StatusOK, nil)
	useTestSpeedtestEndpoint(t, srv)

	e := &Examiner{DoSpeedtest: true, SpeedtestKbAmount: 10, SpeedtestTimeout: 1}
	r := Result{Status: "passed"}

	start := time.Now()
	e.runSpeedtest(context.Background(), latencyBudgetClient(srv, 5*time.Second), &r)
	elapsed := time.Since(start)

	if r.Reason != "" {
		t.Errorf("Reason = %q, want none: a transfer cut by the budget is measured, not failed", r.Reason)
	}
	if r.DownloadSpeed <= 0 {
		t.Errorf("DownloadSpeed = %v, want > 0 from the bytes received within the budget", r.DownloadSpeed)
	}
	if r.UploadSpeed <= 0 {
		t.Errorf("UploadSpeed = %v, want > 0 from the bytes sent within the budget", r.UploadSpeed)
	}
	// Two directions, ~1s each, versus 3s per direction if unbounded.
	if elapsed > 4*time.Second {
		t.Errorf("runSpeedtest took %v, want it bounded near 2s", elapsed)
	}
}

// throttledServer streams /__down forever at roughly `chunk` bytes per `tick`
// and drains /__up at the same pace, so neither direction can finish within a
// short budget. This is the shape of a slow real-world proxy.
func throttledServer(t *testing.T, chunk int, tick time.Duration) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/__down":
			w.WriteHeader(http.StatusOK)
			w.(http.Flusher).Flush()
			buf := make([]byte, chunk)
			for r.Context().Err() == nil {
				if _, err := w.Write(buf); err != nil {
					return
				}
				w.(http.Flusher).Flush()
				time.Sleep(tick)
			}
		case "/__up":
			buf := make([]byte, chunk)
			for r.Context().Err() == nil {
				if _, err := io.ReadFull(r.Body, buf); err != nil {
					break
				}
				time.Sleep(tick)
			}
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// A link too slow to move --amount within the budget must still get a speed:
// the throughput achieved inside the window, in both directions.
func TestRunSpeedtestMeasuresSlowLinkWithinBudget(t *testing.T) {
	srv := throttledServer(t, 20_000, 50*time.Millisecond) // ~3.2 Mbps
	useTestSpeedtestEndpoint(t, srv)

	// 50 MB per direction can never complete in a 1s budget.
	e := &Examiner{DoSpeedtest: true, SpeedtestKbAmount: 50_000, SpeedtestTimeout: 1}
	r := Result{Status: "passed"}

	start := time.Now()
	e.runSpeedtest(context.Background(), latencyBudgetClient(srv, 5*time.Second), &r)
	elapsed := time.Since(start)

	if r.Reason != "" {
		t.Errorf("Reason = %q, want none", r.Reason)
	}
	if r.DownloadSpeed <= 0 {
		t.Errorf("DownloadSpeed = %v, want > 0", r.DownloadSpeed)
	}
	if r.UploadSpeed <= 0 {
		t.Errorf("UploadSpeed = %v, want > 0", r.UploadSpeed)
	}
	if elapsed > 4*time.Second {
		t.Errorf("runSpeedtest took %v, want it bounded near 2s", elapsed)
	}
}

// A download that never delivers a byte inside the budget is a real failure and
// must still be reported as one, not as a made-up speed.
func TestRunSpeedtestStalledDownloadStillFails(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-r.Context().Done() // headers only, then stall until the client gives up
	}))
	t.Cleanup(srv.Close)
	useTestSpeedtestEndpoint(t, srv)

	e := &Examiner{DoSpeedtest: true, SpeedtestKbAmount: 10, SpeedtestTimeout: 1}
	r := Result{Status: "passed"}
	e.runSpeedtest(context.Background(), latencyBudgetClient(srv, 5*time.Second), &r)

	if !strings.Contains(r.Reason, "speedtest_download_failed") {
		t.Errorf("Reason = %q, want a download failure for a stalled transfer", r.Reason)
	}
	if r.DownloadSpeed != 0 {
		t.Errorf("DownloadSpeed = %v, want 0 when nothing arrived", r.DownloadSpeed)
	}
}

func TestSpeedtestTimeoutDefault(t *testing.T) {
	if got := (&Examiner{}).speedtestTimeout(); got != defaultSpeedtestTimeout {
		t.Errorf("speedtestTimeout() = %v, want %v", got, defaultSpeedtestTimeout)
	}
	if got := (&Examiner{SpeedtestTimeout: 45}).speedtestTimeout(); got != 45*time.Second {
		t.Errorf("speedtestTimeout() = %v, want 45s", got)
	}
}

func TestMbps(t *testing.T) {
	// 1 MB in 1s = 8 Mbps.
	got, err := mbps(1_000_000, time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 8 {
		t.Errorf("mbps() = %v, want 8", got)
	}
	if _, err := mbps(0, time.Second); err == nil {
		t.Error("expected an error when no bytes moved")
	}
	if _, err := mbps(1000, 0); err == nil {
		t.Error("expected an error for a zero duration")
	}
}

// The upload body must be replayable, otherwise a redirected or retried POST
// sends nothing.
func TestMakeUploadHTTPRequestBodyIsReplayable(t *testing.T) {
	req := speedtest.MakeUploadHTTPRequest(false, 2048)
	if req.GetBody == nil {
		t.Fatal("GetBody is nil")
	}
	first, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	replay, err := req.GetBody()
	if err != nil {
		t.Fatalf("GetBody: %v", err)
	}
	second, err := io.ReadAll(replay)
	if err != nil {
		t.Fatalf("read replayed body: %v", err)
	}
	if len(first) != 2048 || len(second) != 2048 {
		t.Errorf("body lengths = %d/%d, want 2048/2048", len(first), len(second))
	}
}
