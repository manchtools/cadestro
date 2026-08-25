package firewall

import "testing"

func TestNftParseRules_DecodesSourceDest(t *testing.T) {
	cases := []struct {
		name       string
		json       string
		wantSource string
		wantDest   string
	}{
		{
			name:       "source CIDR (prefix form)",
			json:       `{"nftables":[{"rule":{"comment":"r1","expr":[{"match":{"op":"==","left":{"payload":{"protocol":"ip","field":"saddr"}},"right":{"prefix":{"addr":"10.0.0.0","len":24}}}},{"drop":null}]}}]}`,
			wantSource: "10.0.0.0/24",
		},
		{
			name:       "source bare address (string form)",
			json:       `{"nftables":[{"rule":{"comment":"r2","expr":[{"match":{"op":"==","left":{"payload":{"protocol":"ip","field":"saddr"}},"right":"10.0.0.1"}},{"drop":null}]}}]}`,
			wantSource: "10.0.0.1",
		},
		{
			name:     "dest CIDR",
			json:     `{"nftables":[{"rule":{"comment":"r3","expr":[{"match":{"op":"==","left":{"payload":{"protocol":"ip","field":"daddr"}},"right":{"prefix":{"addr":"192.168.0.0","len":16}}}},{"accept":null}]}}]}`,
			wantDest: "192.168.0.0/16",
		},
		{
			name:       "ipv6 source CIDR",
			json:       `{"nftables":[{"rule":{"comment":"r4","expr":[{"match":{"op":"==","left":{"payload":{"protocol":"ip6","field":"saddr"}},"right":{"prefix":{"addr":"2001:db8::","len":32}}}},{"drop":null}]}}]}`,
			wantSource: "2001:db8::/32",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rules, err := nftParseRules([]byte(tc.json))
			if err != nil {
				t.Fatalf("nftParseRules: %v", err)
			}
			if len(rules) != 1 {
				t.Fatalf("got %d rules, want 1", len(rules))
			}
			if rules[0].Source != tc.wantSource {
				t.Errorf("Source = %q, want %q", rules[0].Source, tc.wantSource)
			}
			if rules[0].Dest != tc.wantDest {
				t.Errorf("Dest = %q, want %q", rules[0].Dest, tc.wantDest)
			}
		})
	}
}
