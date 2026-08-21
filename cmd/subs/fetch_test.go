package subs

import (
	"database/sql"
	"testing"

	"github.com/lilendian0x00/xray-knife/v11/pkg/core"
)

func newTestFetchCommand() *FetchCommand {
	return &FetchCommand{
		config: &FetchConfig{},
		core:   core.NewAutomaticCore(false, false),
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
