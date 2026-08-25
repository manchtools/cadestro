package remote

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestHTTPFetch_WriteIntoReadOnlyDir_Errors(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}
	ro := t.TempDir()
	if err := os.Chmod(ro, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(ro, 0o700) })

	fix := newHTTPFixture(t, []byte("payload"), "etag-1")
	src, err := NewHTTP(HTTPConfig{URL: fix.srv.URL})
	if err != nil {
		t.Fatalf("NewHTTP: %v", err)
	}

	if _, ferr := src.Fetch(context.Background(), filepath.Join(ro, "sub", "out")); ferr == nil {
		t.Error("Fetch into a read-only dir returned nil error")
	}
}

func TestS3Fetch_WriteIntoReadOnlyDir_Errors(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}
	ro := t.TempDir()
	if err := os.Chmod(ro, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(ro, 0o700) })

	fix := newS3Fixture(t, "bucket", "key", []byte("payload"), "etag-1")
	src, err := NewS3(S3Config{Endpoint: fix.srv.URL, Bucket: "bucket", Key: "key"})
	if err != nil {
		t.Fatalf("NewS3: %v", err)
	}
	if _, ferr := src.Fetch(context.Background(), filepath.Join(ro, "sub", "out")); ferr == nil {
		t.Error("S3 Fetch into a read-only dir returned nil error")
	}
}

func TestRestoreUntracked_WriteError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}
	ro := t.TempDir()
	if err := os.Chmod(ro, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(ro, 0o700) })

	snap := []untrackedFile{{relPath: "sub/u", body: []byte("x"), mode: 0o600}}
	if err := restoreUntracked(ro, snap); err == nil {
		t.Error("restoreUntracked into a read-only dir returned nil error")
	}
}
