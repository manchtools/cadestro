package remote

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTLSFixture(t *testing.T, payload []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"v1"`)
		_, _ = w.Write(payload)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestHTTPFetch_InjectedClient_ReachesTLSServer(t *testing.T) {
	payload := []byte("delivered over TLS")
	srv := newTLSFixture(t, payload)
	dest := filepath.Join(t.TempDir(), "file")
	recordDestUnder(t, dest)

	src, err := NewHTTP(HTTPConfig{URL: srv.URL + "/file", Client: srv.Client()})
	if err != nil {
		t.Fatalf("NewHTTP: %v", err)
	}
	res, err := src.Fetch(context.Background(), dest)
	if err != nil {
		t.Fatalf("Fetch over injected TLS client: %v", err)
	}
	if !res.Changed || res.BytesWritten != int64(len(payload)) {
		t.Fatalf("Result = %+v; want Changed with %d bytes", res, len(payload))
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("dest = %q; want %q", got, payload)
	}
}

func TestHTTPFetch_InjectedClient_StillEnforcesChecksum(t *testing.T) {
	srv := newTLSFixture(t, []byte("real body"))
	dest := filepath.Join(t.TempDir(), "file")
	recordDestUnder(t, dest)

	src, err := NewHTTP(HTTPConfig{
		URL:            srv.URL + "/file",
		ChecksumSHA256: strings.Repeat("0", 64),
		Client:         srv.Client(),
	})
	if err != nil {
		t.Fatalf("NewHTTP: %v", err)
	}
	if _, err := src.Fetch(context.Background(), dest); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("Fetch err = %v; want ErrIntegrity even with an injected client", err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("dest must not exist after checksum failure; stat err = %v", err)
	}
}

func TestNewHTTP_NilClientUsesDefault(t *testing.T) {
	src, err := NewHTTP(HTTPConfig{URL: "https://example.com/x"})
	if err != nil {
		t.Fatalf("NewHTTP: %v", err)
	}
	if src.(*httpSource).client == nil {
		t.Fatal("nil Client must fall back to the default client, got nil")
	}
}
