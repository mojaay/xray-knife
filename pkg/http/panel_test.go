package http

import (
	"strings"
	"testing"
)

func TestStatusOK(t *testing.T) {
	tests := []struct {
		name   string
		code   int
		expect int
		want   bool
	}{
		{"any-2xx accepts 200", 200, 0, true},
		{"any-2xx accepts 204", 204, 0, true},
		{"any-2xx accepts 299", 299, 0, true},
		{"any-2xx rejects 301", 301, 0, false},
		{"any-2xx rejects 403", 403, 0, false},
		{"any-2xx rejects 502", 502, 0, false},
		{"exact 204 accepts 204", 204, 204, true},
		{"exact 204 rejects 200 (captive portal)", 200, 204, false},
		{"exact 200 rejects 204", 204, 200, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := statusOK(tc.code, tc.expect); got != tc.want {
				t.Errorf("statusOK(%d, %d) = %v, want %v", tc.code, tc.expect, got, tc.want)
			}
		})
	}
}

func TestExpectedStatusFor(t *testing.T) {
	if got := expectedStatusFor("https://www.gstatic.com/generate_204"); got != 204 {
		t.Errorf("generate_204 URL: got %d, want 204", got)
	}
	if got := expectedStatusFor("https://cloudflare.com/cdn-cgi/trace"); got != 0 {
		t.Errorf("non-204 URL: got %d, want 0", got)
	}
}

func TestPassesThreshold(t *testing.T) {
	tests := []struct {
		name      string
		successes int
		total     int
		threshold float64
		want      bool
	}{
		{"all pass at 1.0", 4, 4, 1.0, true},
		{"3 of 4 fails at 1.0", 3, 4, 1.0, false},
		{"3 of 4 passes at 0.75", 3, 4, 0.75, true},
		{"3 of 4 fails at 0.8", 3, 4, 0.8, false},
		{"1 of 4 fails at 0.75", 1, 4, 0.75, false},
		{"single endpoint pass", 1, 1, 1.0, true},
		{"single endpoint fail", 0, 1, 1.0, false},
		{"zero total", 0, 0, 1.0, false},
		{"half at 0.5", 2, 4, 0.5, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := passesThreshold(tc.successes, tc.total, tc.threshold); got != tc.want {
				t.Errorf("passesThreshold(%d, %d, %g) = %v, want %v",
					tc.successes, tc.total, tc.threshold, got, tc.want)
			}
		})
	}
}

func TestExaminerChecksFallbackToSingleEndpoint(t *testing.T) {
	e := &Examiner{
		TestEndpoint:           "https://example.com/",
		TestEndpointHttpMethod: "HEAD",
	}
	checks := e.checks()
	if len(checks) != 1 {
		t.Fatalf("expected 1 check in fallback, got %d", len(checks))
	}
	if checks[0].URL != "https://example.com/" || checks[0].Method != "HEAD" {
		t.Errorf("unexpected fallback check: %+v", checks[0])
	}
}

func TestExaminerChecksFillsDefaults(t *testing.T) {
	e := &Examiner{
		TestEndpoints: []EndpointCheck{
			{URL: "https://www.gstatic.com/generate_204"}, // no method, no expect
			{URL: "https://cloudflare.com/cdn-cgi/trace", Method: "POST", ExpectStatus: 201},
		},
	}
	checks := e.checks()
	if len(checks) != 2 {
		t.Fatalf("expected 2 checks, got %d", len(checks))
	}
	// Defaults inferred for the first entry.
	if checks[0].Method != "GET" {
		t.Errorf("default method: got %q, want GET", checks[0].Method)
	}
	if checks[0].ExpectStatus != 204 {
		t.Errorf("inferred expect for generate_204: got %d, want 204", checks[0].ExpectStatus)
	}
	// Explicit values preserved for the second entry.
	if checks[1].Method != "POST" || checks[1].ExpectStatus != 201 {
		t.Errorf("explicit values not preserved: %+v", checks[1])
	}
}

func TestCheckPresetsAreWellFormed(t *testing.T) {
	if len(CheckPresets) == 0 {
		t.Fatal("no presets defined")
	}
	for name, panel := range CheckPresets {
		if len(panel) == 0 {
			t.Errorf("preset %q is empty", name)
		}
		for i, c := range panel {
			if c.URL == "" {
				t.Errorf("preset %q entry %d has empty URL", name, i)
			}
			if strings.Contains(c.URL, "generate_204") && c.ExpectStatus != 204 {
				t.Errorf("preset %q entry %d is a generate_204 URL but expects %d", name, i, c.ExpectStatus)
			}
		}
	}
}

func TestCloudflarePresetAndDefault(t *testing.T) {
	const want = "https://cloudflare.com/cdn-cgi/trace"

	cf, ok := CheckPresets["cloudflare"]
	if !ok {
		t.Fatal("cloudflare preset missing")
	}
	if len(cf) != 1 || cf[0].URL != want {
		t.Errorf("cloudflare preset = %+v, want single endpoint %q", cf, want)
	}

	e, err := NewExaminer(Options{})
	if err != nil {
		t.Fatalf("NewExaminer: %v", err)
	}
	if e.TestEndpoint != want {
		t.Errorf("default TestEndpoint = %q, want %q", e.TestEndpoint, want)
	}

	gs, ok := CheckPresets["gstatic"]
	if !ok {
		t.Fatal("gstatic preset missing")
	}
	if len(gs) != 1 || gs[0].URL != "https://www.gstatic.com/generate_204" || gs[0].ExpectStatus != 204 {
		t.Errorf("gstatic preset = %+v, want single generate_204 endpoint expecting 204", gs)
	}
}

func TestPresetNamesSorted(t *testing.T) {
	names := PresetNames()
	if len(names) != len(CheckPresets) {
		t.Fatalf("PresetNames returned %d, want %d", len(names), len(CheckPresets))
	}
	for i := 1; i < len(names); i++ {
		if names[i-1] > names[i] {
			t.Errorf("PresetNames not sorted: %v", names)
			break
		}
	}
}
