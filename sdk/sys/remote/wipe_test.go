package remote

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestHTTPWipe_RemovesRecordedDest(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "tree")
	if err := os.MkdirAll(filepath.Join(dest, "sub"), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dest, "sub", "x"), []byte("y"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	recordDestUnder(t, dest)

	src, _ := NewHTTP(HTTPConfig{URL: "https://example.test/x"})
	if err := src.Wipe(context.Background(), dest); err != nil {
		t.Fatalf("Wipe: %v", err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("dest still exists after Wipe; stat err = %v", err)
	}
}

func TestHTTPWipe_RefusesProtectedPaths(t *testing.T) {
	src, _ := NewHTTP(HTTPConfig{URL: "https://example.test/x"})
	for _, p := range []string{"/etc", "/var/log", "/"} {
		t.Run("dest="+p, func(t *testing.T) {
			if err := src.Wipe(context.Background(), p); !errors.Is(err, ErrUnsafeDestination) {
				t.Fatalf("Wipe(%q) = %v; want errors.Is(..., ErrUnsafeDestination)", p, err)
			}
		})
	}
}

func TestHTTPWipe_NoOpWhenDestMissing(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "never-existed")
	recordDestUnder(t, dest)

	src, _ := NewHTTP(HTTPConfig{URL: "https://example.test/x"})
	if err := src.Wipe(context.Background(), dest); err != nil {
		t.Fatalf("Wipe on missing dest err = %v; want nil", err)
	}
}

func TestHTTPWipe_AllowsManagedRootsWithoutRecording(t *testing.T) {

	src, _ := NewHTTP(HTTPConfig{URL: "https://example.test/x"})
	err := src.Wipe(context.Background(), "/var/lib/cadestro/test-wipe-allowance")
	if errors.Is(err, ErrUnsafeDestination) {
		t.Fatalf("Wipe under managed root returned ErrUnsafeDestination: %v", err)
	}
}
