package remote

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("injected read error") }

func TestStreamToTmp_CopyError(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out")
	if _, _, _, err := streamToTmp(out, errReader{}, 1<<20); err == nil {
		t.Error("streamToTmp with a failing reader returned nil error")
	}
}

func TestWriteTarEntry_CopyError(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out")
	if _, err := writeTarEntry(out, errReader{}, 0o644); err == nil {
		t.Error("writeTarEntry with a failing reader returned nil error")
	}
}

func TestGitFetch_UnreachableURL(t *testing.T) {
	src, err := NewGit(GitConfig{URL: "https://127.0.0.1:1/nope.git", Ref: "main"})
	if err != nil {
		t.Fatalf("NewGit: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if _, ferr := src.Fetch(ctx, filepath.Join(t.TempDir(), "dest")); ferr == nil {
		t.Error("Fetch from an unreachable git endpoint returned nil error")
	}
}

func TestS3Fetch_GetError(t *testing.T) {
	fix := newS3Fixture(t, "bucket", "key", []byte("payload"), "etag-1")
	fix.getErr = 500
	src, err := NewS3(S3Config{Endpoint: fix.srv.URL, Bucket: "bucket", Key: "key"})
	if err != nil {
		t.Fatalf("NewS3: %v", err)
	}
	if _, ferr := src.Fetch(context.Background(), filepath.Join(t.TempDir(), "out")); ferr == nil {
		t.Error("S3 Fetch with a 500 on GET returned nil error")
	}
}
