package cfscanner

import (
	"strings"
	"testing"
)

func TestValidateConfigLink(t *testing.T) {
	cases := []struct {
		name    string
		link    string
		wantErr bool
	}{
		{"empty is fine", "", false},
		{"vless link", "vless://uuid@host:443?security=tls", false},
		{"v10 speedtest-top value", "10", true},
		{"bare host", "example.com", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateConfigLink(tc.link)
			if tc.wantErr && err == nil {
				t.Fatalf("validateConfigLink(%q) = nil, want error", tc.link)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("validateConfigLink(%q) = %v, want nil", tc.link, err)
			}
		})
	}
}

func TestValidateConfigLinkMentionsSpeedtestTop(t *testing.T) {
	err := validateConfigLink("10")
	if err == nil {
		t.Fatal("expected an error for a bare integer")
	}
	if !strings.Contains(err.Error(), "--speedtest-top") {
		t.Fatalf("error %q should point users at --speedtest-top", err)
	}
}
