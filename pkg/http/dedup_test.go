package http

import (
	"encoding/base64"
	"testing"

	"github.com/lilendian0x00/xray-knife/v11/pkg/core"
)

func b64Vmess(json string) string {
	return "vmess://" + base64.StdEncoding.EncodeToString([]byte(json))
}

func TestSemanticDeduplicateLinks(t *testing.T) {
	c := core.CoreFactoryWith(core.XrayCoreType, core.FactoryOptions{})
	uuid := "a1a1a1a1-b2b2-c3c3-d4d4-e5e5e5e5e5e5"

	// Same vmess server, JSON keys in a different order and a different remark:
	// must collapse to one.
	vmessA := b64Vmess(`{"v":"2","add":"1.2.3.4","port":"443","id":"` + uuid + `","aid":"0","net":"ws","tls":"tls","host":"a.com","path":"/x","ps":"US-Fast"}`)
	vmessB := b64Vmess(`{"ps":"Server-01","path":"/x","host":"a.com","tls":"tls","net":"ws","aid":"0","id":"` + uuid + `","port":"443","add":"1.2.3.4","v":"2"}`)

	cases := []struct {
		name      string
		links     []string
		wantKept  int
		wantRmvd  int
		wantFirst []string // expected surviving links, in order
	}{
		{
			name:      "vless same server different remark",
			links:     []string{vless443(uuid, "tls", "🇺🇸 US-Fast"), vless443(uuid, "tls", "Server-01")},
			wantKept:  1,
			wantRmvd:  1,
			wantFirst: []string{vless443(uuid, "tls", "🇺🇸 US-Fast")},
		},
		{
			name: "vless reordered query params collapse",
			links: []string{
				"vless://" + uuid + "@1.2.3.4:443?type=ws&security=tls&host=a.com&path=%2Fx#r1",
				"vless://" + uuid + "@1.2.3.4:443?security=tls&host=a.com&type=ws&path=%2Fx#r2",
			},
			wantKept: 1,
			wantRmvd: 1,
		},
		{
			name:     "vmess reordered json keys collapse",
			links:    []string{vmessA, vmessB},
			wantKept: 1,
			wantRmvd: 1,
		},
		{
			name: "same address:port different protocol kept",
			links: []string{
				"vless://" + uuid + "@1.2.3.4:443?encryption=none&security=tls&type=tcp#v",
				"trojan://password@1.2.3.4:443?security=tls&type=tcp#t",
			},
			wantKept: 2,
			wantRmvd: 0,
		},
		{
			name: "same server different credential kept",
			links: []string{
				"vless://11111111-1111-1111-1111-111111111111@1.2.3.4:443?encryption=none&security=tls&type=tcp#a",
				"vless://22222222-2222-2222-2222-222222222222@1.2.3.4:443?encryption=none&security=tls&type=tcp#b",
			},
			wantKept: 2,
			wantRmvd: 0,
		},
		{
			name: "different sni kept",
			links: []string{
				"vless://" + uuid + "@1.2.3.4:443?encryption=none&security=tls&type=tcp&sni=one.com#a",
				"vless://" + uuid + "@1.2.3.4:443?encryption=none&security=tls&type=tcp&sni=two.com#b",
			},
			wantKept: 2,
			wantRmvd: 0,
		},
		{
			name:     "unparseable identical collapse",
			links:    []string{"not-a-link", "not-a-link"},
			wantKept: 1,
			wantRmvd: 1,
		},
		{
			name:     "unparseable distinct kept",
			links:    []string{"garbage-one", "garbage-two"},
			wantKept: 2,
			wantRmvd: 0,
		},
		{
			name:     "blank lines ignored",
			links:    []string{"", "   ", vless443(uuid, "tls", "only")},
			wantKept: 1,
			wantRmvd: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, removed := SemanticDeduplicateLinks(c, tc.links)
			if len(got) != tc.wantKept {
				t.Errorf("kept %d, want %d (got %v)", len(got), tc.wantKept, got)
			}
			if removed != tc.wantRmvd {
				t.Errorf("removed %d, want %d", removed, tc.wantRmvd)
			}
			for i, w := range tc.wantFirst {
				if i >= len(got) || got[i] != w {
					t.Errorf("survivor[%d] = %q, want %q", i, safeIdx(got, i), w)
				}
			}
		})
	}
}

func vless443(uuid, security, remark string) string {
	return "vless://" + uuid + "@1.2.3.4:443?encryption=none&security=" + security + "&host=a.com&path=%2Fx&type=ws#" + remark
}

func safeIdx(s []string, i int) string {
	if i < 0 || i >= len(s) {
		return "<none>"
	}
	return s[i]
}
