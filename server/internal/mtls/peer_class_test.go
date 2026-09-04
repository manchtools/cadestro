package mtls

import (
	"crypto/x509"
	"net/url"
	"testing"
)

func mustURL(t *testing.T, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return u
}

func TestPeerClassFromCert_Roundtrip(t *testing.T) {
	u, err := PeerClassURI(PeerClassAgent)
	if err != nil {
		t.Fatalf("PeerClassURI(%q): %v", PeerClassAgent, err)
	}
	cert := &x509.Certificate{URIs: []*url.URL{u}}
	got, err := PeerClassFromCert(cert)
	if err != nil {
		t.Fatalf("PeerClassFromCert: %v", err)
	}
	if got != PeerClassAgent {
		t.Errorf("got %q, want %q", got, PeerClassAgent)
	}
}

func TestPeerClassFromCert_Errors(t *testing.T) {
	cases := map[string]*x509.Certificate{
		"nil cert":      nil,
		"no URI SAN":    {},
		"wrong scheme":  {URIs: []*url.URL{mustURL(t, "https://cadestro/agent")}},
		"wrong host":    {URIs: []*url.URL{mustURL(t, "spiffe://other/agent")}},
		"unknown class": {URIs: []*url.URL{mustURL(t, "spiffe://cadestro/admin")}},
		"empty class":   {URIs: []*url.URL{mustURL(t, "spiffe://cadestro/")}},
		"multi-class": {URIs: []*url.URL{
			mustURL(t, "spiffe://cadestro/agent"),
			mustURL(t, "spiffe://cadestro/gateway"),
		}},
	}
	for name, cert := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := PeerClassFromCert(cert); err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}
