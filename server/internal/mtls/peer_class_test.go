package mtls

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
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
	for _, class := range []PeerClass{PeerClassAgent, PeerClassControl} {
		t.Run(string(class), func(t *testing.T) {
			u, err := PeerClassURI(class)
			if err != nil {
				t.Fatalf("PeerClassURI(%q): %v", class, err)
			}
			cert := &x509.Certificate{URIs: []*url.URL{u}}
			got, err := PeerClassFromCert(cert)
			if err != nil {
				t.Fatalf("PeerClassFromCert: %v", err)
			}
			if got != class {
				t.Errorf("got %q, want %q", got, class)
			}
		})
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

func TestRequirePeerClass_AllowsAllowedRejectsOthers(t *testing.T) {
	discardLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := RequirePeerClass(discardLogger, PeerClassAgent)(next)

	call := func(class PeerClass) int {
		req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(""))
		u, _ := PeerClassURI(class)
		req.TLS = &tls.ConnectionState{
			PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{u}}},
		}
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		return rr.Code
	}

	if code := call(PeerClassAgent); code != http.StatusOK {
		t.Errorf("allowed agent class got %d, want 200", code)
	}
	if code := call(PeerClassControl); code != http.StatusForbidden {
		t.Errorf("disallowed control class got %d, want 403 — a control-class cert must not reach the agent listener", code)
	}
}

func TestRequirePeerClass_HealthBypass(t *testing.T) {
	discardLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := RequirePeerClass(discardLogger, PeerClassAgent)(next)

	for _, path := range []string{"/health", "/ready"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			if rr.Code != http.StatusOK {
				t.Errorf("health bypass: got %d", rr.Code)
			}
		})
	}
}

func TestRequirePeerClass_RejectsMissingTLS(t *testing.T) {
	discardLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := RequirePeerClass(discardLogger, PeerClassControl)(http.NotFoundHandler())
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("missing TLS: got %d, want 401", rr.Code)
	}
}
