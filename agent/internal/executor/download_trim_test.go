package executor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestFetchArtifact_AcceptsWhitespacePaddedURL(t *testing.T) {
	const body = "artifact-bytes"
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/app" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	withRemoteTestClient(t, srv)

	dest := filepath.Join(t.TempDir(), "app")
	padded := "  " + srv.URL + "/app\n"
	if err := fetchArtifact(context.Background(), padded, dest, "", "0644", redirectForArtifact("")); err != nil {
		t.Fatalf("fetchArtifact on a whitespace-padded URL: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(got) != body {
		t.Errorf("dest content = %q, want %q", got, body)
	}
}
