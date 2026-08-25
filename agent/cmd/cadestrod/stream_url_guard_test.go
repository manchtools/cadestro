package main

import "testing"

func TestStreamURLGuard_RejectsNonHTTPS(t *testing.T) {
	cases := []struct {
		name    string
		addr    string
		wantErr bool
	}{

		{"https host with port", "https://gw.example:443", false},
		{"https host no port", "https://gw.example", false},
		{"https host with path", "https://gw.example/connect", false},

		{"lowercase http", "http://attacker", true},
		{"uppercase HTTP", "HTTP://attacker", true},
		{"mixed-case Https host", "Https://x", false},
		{"mixed-case HTTP variant", "HtTp://x", true},
		{"opaque https", "https:foo", true},
		{"https no host", "https:", true},
		{"ftp scheme", "ftp://x", true},
		{"h2c scheme", "h2c://x", true},
		{"empty", "", true},
		{"leading whitespace http", " http://x", true},
		{"leading whitespace https", " https://x", false},
		{"scheme-less host", "gw.example:443", true},
		{"bare host", "gw.example", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := requireHTTPSAgentAddr(tc.addr)
			if tc.wantErr && err == nil {
				t.Fatalf("requireHTTPSAgentAddr(%q) = nil, want error", tc.addr)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("requireHTTPSAgentAddr(%q) = %v, want nil", tc.addr, err)
			}
		})
	}
}
