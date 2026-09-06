package subs

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lilendian0x00/xray-knife/v11/pkg/core"
	"github.com/lilendian0x00/xray-knife/v11/pkg/subscription"
)

func newTestFetchCommand() *FetchCommand {
	return &FetchCommand{
		config: &FetchConfig{},
		core:   core.NewAutomaticCore(false, false),
	}
}

func TestFetchSourceAppliesConfiguredLimits(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("socks://one:1080\nsocks://two:1080"))
	}))
	defer server.Close()
	fc := newTestFetchCommand()
	fc.config.MaxLinks = 1
	if _, err := fc.fetchSource(context.Background(), &Subscription{Url: server.URL}); !errors.Is(err, subscription.ErrTooManyLinks) {
		t.Fatalf("CLI did not pass the link limit: %v", err)
	}
	fc.config.MaxLinks = 2
	if links, err := fc.fetchSource(context.Background(), &Subscription{Url: server.URL}); err != nil || len(links) != 2 {
		t.Fatalf("raised CLI limit not honored: %d links, %v", len(links), err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := fc.fetchSource(ctx, &Subscription{Url: server.URL}); !errors.Is(err, context.Canceled) {
		t.Fatalf("CLI did not propagate cancellation: %v", err)
	}
}

func TestFetchLimitFlags(t *testing.T) {
	fc := newTestFetchCommand()
	cmd := fc.createCommand()
	if err := cmd.ParseFlags([]string{"--url", "https://example.com/sub", "--max-links", "200000", "--max-bytes", "134217728", "--fetch-timeout", "2m"}); err != nil {
		t.Fatal(err)
	}
	if err := fc.validateFlags(cmd, nil); err != nil {
		t.Fatal(err)
	}
	if fc.config.MaxLinks != 200000 || fc.config.MaxBytes != 134217728 || fc.config.Timeout != 2*time.Minute {
		t.Fatal("fetch limits were not configured")
	}
	fc.config.MaxLinks = 0
	if err := fc.validateFlags(cmd, nil); err == nil {
		t.Fatal("zero CLI link limit accepted")
	}
}

func TestParseLinksCountsUnparsable(t *testing.T) {
	fc := newTestFetchCommand()

	links := []string{
		"vless://11111111-1111-1111-1111-111111111111@example.com:443?security=tls&type=tcp#ok",
		"this-is-not-a-config-link",
		"",
		"also://nonsense-protocol",
	}

	configs, unparsable := fc.parseLinks(links, sql.NullInt64{})

	if len(configs) != 3 {
		t.Fatalf("len(configs) = %d, want 3 (blank line skipped)", len(configs))
	}
	if unparsable != 2 {
		t.Fatalf("unparsable = %d, want 2", unparsable)
	}
}

func TestParseLinksAllGoodReportsZero(t *testing.T) {
	fc := newTestFetchCommand()

	links := []string{
		"vless://11111111-1111-1111-1111-111111111111@example.com:443?security=tls&type=tcp#ok",
	}

	configs, unparsable := fc.parseLinks(links, sql.NullInt64{})

	if len(configs) != 1 {
		t.Fatalf("len(configs) = %d, want 1", len(configs))
	}
	if unparsable != 0 {
		t.Fatalf("unparsable = %d, want 0", unparsable)
	}
}
