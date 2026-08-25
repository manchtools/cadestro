package remote

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestApplyMode(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "x")
	if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := applyMode(f, "", "", ""); err != nil {
		t.Errorf("applyMode(no-op) = %v, want nil", err)
	}

	if err := applyMode(f, "0640", "", ""); err != nil {
		t.Fatalf("applyMode(0640): %v", err)
	}
	if info, _ := os.Stat(f); info.Mode().Perm() != 0o640 {
		t.Errorf("perm = %o, want 0640", info.Mode().Perm())
	}

	if err := applyMode(f, "not-octal", "", ""); err == nil {
		t.Error("applyMode(invalid mode) = nil, want error")
	}
}

func TestHTTPFetch_Non2xx_Errors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	src, err := NewHTTP(HTTPConfig{URL: srv.URL + "/f"})
	if err != nil {
		t.Fatalf("NewHTTP: %v", err)
	}
	dest := filepath.Join(t.TempDir(), "out")
	if _, err := src.Fetch(context.Background(), dest); err == nil {
		t.Error("Fetch on a 503 returned nil error")
	}
	if _, statErr := os.Stat(dest); statErr == nil {
		t.Error("Fetch on a 503 left a destination file behind")
	}
}

func TestS3Fetch_ErrorStatuses(t *testing.T) {
	cases := []struct {
		code        int
		wantInvalid bool
	}{
		{http.StatusForbidden, true},
		{http.StatusNotFound, false},
		{http.StatusInternalServerError, false},
	}
	for _, c := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(c.code)
		}))
		src, err := NewS3(S3Config{Endpoint: srv.URL, Bucket: "b", Key: "k"})
		if err != nil {
			srv.Close()
			t.Fatalf("NewS3: %v", err)
		}
		dest := filepath.Join(t.TempDir(), "out")
		_, ferr := src.Fetch(context.Background(), dest)
		srv.Close()
		if ferr == nil {
			t.Errorf("S3 Fetch on %d returned nil error", c.code)
			continue
		}
		if c.wantInvalid && !errors.Is(ferr, ErrInvalidConfig) {
			t.Errorf("S3 Fetch on %d: err = %v, want ErrInvalidConfig", c.code, ferr)
		}
		if _, statErr := os.Stat(dest); statErr == nil {
			t.Errorf("S3 Fetch on %d left a destination file behind", c.code)
		}
	}
}
