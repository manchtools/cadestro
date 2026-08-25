package executor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/sdk/sys/remote"
)

func crossOriginFixture(t *testing.T, payload []byte) string {
	t.Helper()
	srvB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	t.Cleanup(srvB.Close)
	srvA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, srvB.URL+"/file", http.StatusFound)
	}))
	t.Cleanup(srvA.Close)
	return srvA.URL + "/file"
}

func TestFetchArtifact_RedirectPolicy(t *testing.T) {
	prev := remoteHTTPClient
	remoteHTTPClient = nil
	t.Cleanup(func() { remoteHTTPClient = prev })

	payload := []byte("agent binary bytes behind a cross-origin redirect")
	sum := sha256.Sum256(payload)
	checksum := hex.EncodeToString(sum[:])
	url := crossOriginFixture(t, payload)

	dest := filepath.Join(t.TempDir(), "bin")
	if err := fetchArtifact(context.Background(), url, dest, checksum, "0755", remote.RedirectSameOrigin); err == nil {
		t.Fatal("RedirectSameOrigin must refuse the cross-origin redirect, got nil")
	}

	dest2 := filepath.Join(t.TempDir(), "bin")
	if err := fetchArtifact(context.Background(), url, dest2, checksum, "0755", remote.RedirectCrossOrigin); err != nil {
		t.Fatalf("RedirectCrossOrigin must follow the redirect: %v", err)
	}
	got, err := os.ReadFile(dest2)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("dest = %q; want %q", got, payload)
	}
}

func TestUpdateRedirectPolicy(t *testing.T) {
	if got := updateRedirectPolicy(&pb.AgentUpdateParams{AllowRedirect: true}); got != remote.RedirectCrossOrigin {
		t.Fatalf("AllowRedirect=true -> %v; want RedirectCrossOrigin", got)
	}
	if got := updateRedirectPolicy(&pb.AgentUpdateParams{AllowRedirect: false}); got != remote.RedirectSameOrigin {
		t.Fatalf("AllowRedirect=false -> %v; want RedirectSameOrigin", got)
	}
	if got := updateRedirectPolicy(&pb.AgentUpdateParams{}); got != remote.RedirectSameOrigin {
		t.Fatalf("default -> %v; want RedirectSameOrigin", got)
	}
}

func TestRedirectForArtifact(t *testing.T) {
	if got := redirectForArtifact("abc123"); got != remote.RedirectCrossOrigin {
		t.Fatalf("pinned -> %v; want RedirectCrossOrigin", got)
	}
	if got := redirectForArtifact(""); got != remote.RedirectSameOrigin {
		t.Fatalf("unpinned -> %v; want RedirectSameOrigin", got)
	}
}
