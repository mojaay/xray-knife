package proxy

import "testing"

// TestListenPortReachesServiceAsString pins the boundary conversion: the CLI
// takes --port as a uint16 (so a bad value fails at parse time with a clear
// message), while pkg/proxy.Config keeps ListenPort as a string.
func TestListenPortReachesServiceAsString(t *testing.T) {
	p := &parentFlags{listenAddr: "127.0.0.1", listenPort: 1080}

	cfg := buildPkgConfig("inbound", p, nil, nil, nil, nil, nil, nil)

	if cfg.ListenPort != "1080" {
		t.Fatalf("ListenPort = %q, want %q", cfg.ListenPort, "1080")
	}
}

func TestListenPortDefaultConverts(t *testing.T) {
	p := &parentFlags{listenAddr: "127.0.0.1", listenPort: 9999}

	cfg := buildPkgConfig("inbound", p, nil, nil, nil, nil, nil, nil)

	if cfg.ListenPort != "9999" {
		t.Fatalf("ListenPort = %q, want %q", cfg.ListenPort, "9999")
	}
}
