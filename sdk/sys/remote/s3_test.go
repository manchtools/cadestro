package remote

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

type s3Fixture struct {
	srv     *httptest.Server
	bucket  string
	key     string
	body    []byte
	etag    string
	headErr int
	getErr  int
	gets    atomic.Int32
	heads   atomic.Int32
}

func newS3Fixture(t *testing.T, bucket, key string, body []byte, etag string) *s3Fixture {
	t.Helper()
	f := &s3Fixture{bucket: bucket, key: key, body: body, etag: etag}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		want := "/" + f.bucket + "/" + f.key
		if r.URL.Path != want {
			http.Error(w, "wrong path "+r.URL.Path, http.StatusNotFound)
			return
		}
		w.Header().Set("ETag", f.etag)
		switch r.Method {
		case http.MethodHead:
			f.heads.Add(1)
			if r.Header.Get("If-None-Match") == f.etag {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			if f.headErr != 0 {
				w.WriteHeader(f.headErr)
				return
			}
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			f.gets.Add(1)
			if r.Header.Get("If-None-Match") == f.etag {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			if f.getErr != 0 {
				w.WriteHeader(f.getErr)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(f.body)
		default:
			http.Error(w, "method", http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func TestNewS3_RejectsBadConfig(t *testing.T) {
	good := S3Config{Endpoint: "https://s3.example.test", Bucket: "b", Key: "k"}
	for _, tc := range []struct {
		name string
		cfg  S3Config
	}{
		{"empty endpoint", S3Config{Bucket: "b", Key: "k"}},
		{"empty bucket", S3Config{Endpoint: good.Endpoint, Key: "k"}},
		{"empty key", S3Config{Endpoint: good.Endpoint, Bucket: "b"}},
		{"non-http(s) endpoint", S3Config{Endpoint: "ftp://x", Bucket: "b", Key: "k"}},
		{"endpoint with userinfo", S3Config{Endpoint: "https://u:p@s3.test", Bucket: "b", Key: "k"}},
		{"bucket too long", S3Config{Endpoint: good.Endpoint, Bucket: longString(64), Key: "k"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewS3(tc.cfg)
			if !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("NewS3(%s) = %v; want ErrInvalidConfig", tc.name, err)
			}
		})
	}
}

func TestS3Fetch_SingleKey_DownloadsToDest(t *testing.T) {
	payload := []byte("hello s3")
	fix := newS3Fixture(t, "mybucket", "path/to/obj", payload, `"abc"`)
	dest := filepath.Join(t.TempDir(), "obj")
	recordDestUnder(t, dest)

	src, err := NewS3(S3Config{Endpoint: fix.srv.URL, Bucket: "mybucket", Key: "path/to/obj"})
	if err != nil {
		t.Fatalf("NewS3: %v", err)
	}
	res, err := src.Fetch(context.Background(), dest)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !res.Changed || res.BytesWritten != int64(len(payload)) || res.FilesTouched != 1 {
		t.Fatalf("Result = %+v; want Changed=true Bytes=%d FilesTouched=1", res, len(payload))
	}
	if res.Revision == "" {
		t.Fatal("Revision empty")
	}
	body, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(body) != string(payload) {
		t.Fatalf("dest body = %q; want %q", body, payload)
	}
}

func TestS3Fetch_SingleKey_NoOpOnMatchingETag(t *testing.T) {
	fix := newS3Fixture(t, "b", "obj", []byte("data"), `"v1"`)
	dest := filepath.Join(t.TempDir(), "f")
	recordDestUnder(t, dest)

	src, _ := NewS3(S3Config{Endpoint: fix.srv.URL, Bucket: "b", Key: "obj"})

	res1, err := src.Fetch(context.Background(), dest)
	if err != nil {
		t.Fatalf("Fetch #1: %v", err)
	}
	gets1 := fix.gets.Load()

	res2, err := src.Fetch(context.Background(), dest)
	if err != nil {
		t.Fatalf("Fetch #2: %v", err)
	}
	if res2.Changed {
		t.Fatal("second Fetch Changed=true on matching ETag")
	}
	if res2.Revision != res1.Revision {
		t.Fatalf("Revision changed between no-op fetches: %q → %q", res1.Revision, res2.Revision)
	}
	if got := fix.gets.Load(); got != gets1 {
		t.Fatalf("second Fetch issued a GET (count %d → %d)", gets1, got)
	}
}

func TestS3Fetch_403FromHEAD_ReturnsClearError(t *testing.T) {
	fix := newS3Fixture(t, "b", "obj", []byte("x"), `"v1"`)
	fix.getErr = http.StatusForbidden
	dest := filepath.Join(t.TempDir(), "f")
	recordDestUnder(t, dest)

	src, _ := NewS3(S3Config{Endpoint: fix.srv.URL, Bucket: "b", Key: "obj"})
	_, err := src.Fetch(context.Background(), dest)
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("Fetch err = %v; want ErrInvalidConfig", err)
	}
}

func TestS3Fetch_RejectsUnsafeDest(t *testing.T) {
	fix := newS3Fixture(t, "b", "obj", []byte("x"), `"v1"`)
	src, _ := NewS3(S3Config{Endpoint: fix.srv.URL, Bucket: "b", Key: "obj"})
	if _, err := src.Fetch(context.Background(), "rel/path"); !errors.Is(err, ErrUnsafeDestination) {
		t.Fatalf("Fetch err = %v; want ErrUnsafeDestination", err)
	}
	if fix.gets.Load()+fix.heads.Load() != 0 {
		t.Fatal("network was hit before dest validation")
	}
}

func longString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a'
	}
	return string(b)
}
