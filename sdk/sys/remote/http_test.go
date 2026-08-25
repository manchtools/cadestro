package remote

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

type httpFixture struct {
	srv      *httptest.Server
	payload  []byte
	etag     string
	gets     atomic.Int32
	heads    atomic.Int32
	getDelay func(io.Writer)
}

func newHTTPFixture(t *testing.T, payload []byte, etag string) *httpFixture {
	t.Helper()
	f := &httpFixture{payload: payload, etag: etag}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", f.etag)
		switch r.Method {
		case http.MethodHead:
			f.heads.Add(1)
			if r.Header.Get("If-None-Match") == f.etag {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			f.gets.Add(1)
			if r.Header.Get("If-None-Match") == f.etag {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			w.WriteHeader(http.StatusOK)
			if f.getDelay != nil {
				f.getDelay(w)
				return
			}
			_, _ = w.Write(f.payload)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func recordDestUnder(t *testing.T, dest string) {
	t.Helper()
	RecordDest(dest)
	t.Cleanup(func() { forgetDest(dest) })
}

func TestHTTPFetch_DownloadsToDest(t *testing.T) {
	payload := []byte("alpha bravo charlie")
	fix := newHTTPFixture(t, payload, `"v1"`)
	dest := filepath.Join(t.TempDir(), "file")
	recordDestUnder(t, dest)

	src, err := NewHTTP(HTTPConfig{URL: fix.srv.URL + "/file"})
	if err != nil {
		t.Fatalf("NewHTTP: %v", err)
	}
	res, err := src.Fetch(context.Background(), dest)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if !res.Changed {
		t.Fatal("Result.Changed=false on first call")
	}
	if res.BytesWritten != int64(len(payload)) {
		t.Fatalf("BytesWritten = %d; want %d", res.BytesWritten, len(payload))
	}
	if res.FilesTouched != 1 {
		t.Fatalf("FilesTouched = %d; want 1", res.FilesTouched)
	}
	wantSum := sha256.Sum256(payload)
	if res.Digest != hex.EncodeToString(wantSum[:]) {
		t.Fatalf("Digest = %q; want %q", res.Digest, hex.EncodeToString(wantSum[:]))
	}
	if res.Revision == "" {
		t.Fatal("Revision empty; want non-empty drift token")
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("dest content = %q; want %q", got, payload)
	}
}

func TestHTTPFetch_AtomicWrite_LeavesNoTmpBehind(t *testing.T) {
	payload := []byte("zigzag")
	fix := newHTTPFixture(t, payload, `"v1"`)
	dir := t.TempDir()
	dest := filepath.Join(dir, "file")
	recordDestUnder(t, dest)

	src, _ := NewHTTP(HTTPConfig{URL: fix.srv.URL + "/file"})
	if _, err := src.Fetch(context.Background(), dest); err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "file.tmp.") {
			t.Fatalf("leftover tmp file: %s", e.Name())
		}
	}
}

func TestHTTPFetch_ChecksumMismatch(t *testing.T) {
	fix := newHTTPFixture(t, []byte("real body"), `"v1"`)
	dir := t.TempDir()
	dest := filepath.Join(dir, "file")
	recordDestUnder(t, dest)

	src, err := NewHTTP(HTTPConfig{
		URL:            fix.srv.URL + "/file",
		ChecksumSHA256: strings.Repeat("0", 64),
	})
	if err != nil {
		t.Fatalf("NewHTTP: %v", err)
	}
	if _, err := src.Fetch(context.Background(), dest); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("Fetch err = %v; want errors.Is(..., ErrIntegrity)", err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("dest should not exist after checksum failure; stat err = %v", err)
	}
}

func TestHTTPFetch_RespectsMaxBytes(t *testing.T) {
	fix := newHTTPFixture(t, nil, `"v1"`)
	fix.getDelay = func(w io.Writer) {

		buf := make([]byte, 1024)
		for i := 0; i < 10; i++ {
			_, _ = w.Write(buf)
		}
	}
	dir := t.TempDir()
	dest := filepath.Join(dir, "file")
	recordDestUnder(t, dest)

	src, _ := NewHTTP(HTTPConfig{URL: fix.srv.URL + "/file", MaxBytes: 1024})
	if _, err := src.Fetch(context.Background(), dest); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("Fetch err = %v; want errors.Is(..., ErrIntegrity) for size-cap breach", err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("dest should not exist after size-cap failure; stat err = %v", err)
	}
}

func TestHTTPFetch_SecondCallNoOp_WhenETagMatches(t *testing.T) {
	fix := newHTTPFixture(t, []byte("body"), `"v1"`)
	dir := t.TempDir()
	dest := filepath.Join(dir, "file")
	recordDestUnder(t, dest)

	src, _ := NewHTTP(HTTPConfig{URL: fix.srv.URL + "/file"})

	res1, err := src.Fetch(context.Background(), dest)
	if err != nil {
		t.Fatalf("Fetch #1: %v", err)
	}
	if !res1.Changed {
		t.Fatal("first Fetch: Changed=false")
	}
	gets1 := fix.gets.Load()

	res2, err := src.Fetch(context.Background(), dest)
	if err != nil {
		t.Fatalf("Fetch #2: %v", err)
	}
	if res2.Changed {
		t.Fatal("second Fetch with unchanged ETag: Changed=true")
	}
	if res2.Revision != res1.Revision {
		t.Fatalf("Revision changed between no-op fetches: %q vs %q", res1.Revision, res2.Revision)
	}
	if got := fix.gets.Load(); got != gets1 {
		t.Fatalf("second Fetch issued a GET (count went %d → %d)", gets1, got)
	}
}

func TestHTTPFetch_AppliesMode(t *testing.T) {
	fix := newHTTPFixture(t, []byte("data"), `"v1"`)
	dir := t.TempDir()
	dest := filepath.Join(dir, "file")
	recordDestUnder(t, dest)

	src, _ := NewHTTP(HTTPConfig{URL: fix.srv.URL + "/file", Mode: "0640"})
	if _, err := src.Fetch(context.Background(), dest); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o640 {
		t.Fatalf("mode = %v; want 0640", got)
	}
}

func TestHTTPFetch_RejectsCrossHostRedirect(t *testing.T) {
	targetGets := atomic.Int32{}
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetGets.Add(1)
		_, _ = w.Write([]byte("redirected payload"))
	}))
	t.Cleanup(target.Close)

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/payload", http.StatusFound)
	}))
	t.Cleanup(redirector.Close)

	dest := filepath.Join(t.TempDir(), "file")
	recordDestUnder(t, dest)
	src, err := NewHTTP(HTTPConfig{URL: redirector.URL + "/start"})
	if err != nil {
		t.Fatalf("NewHTTP: %v", err)
	}
	if _, err := src.Fetch(context.Background(), dest); err == nil {
		t.Fatal("Fetch followed a cross-host redirect, want a validation error")
	}
	if got := targetGets.Load(); got != 0 {
		t.Fatalf("redirect target received %d request(s); cross-host redirect should be refused before SSRF", got)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("dest should not exist after refused redirect; stat err = %v", err)
	}
}

func TestDefaultHTTPClient_RejectsSchemeDowngradeRedirect(t *testing.T) {
	check := defaultHTTPClient(RedirectSameOrigin).CheckRedirect
	req := func(raw string) *http.Request {
		r, err := http.NewRequest(http.MethodGet, raw, nil)
		if err != nil {
			t.Fatalf("build request %q: %v", raw, err)
		}
		return r
	}
	via := []*http.Request{req("https://host.example/a")}

	if err := check(req("http://host.example/a"), via); err == nil {
		t.Error("CheckRedirect allowed an https->http downgrade on the same host (TLS downgrade)")
	}
	if err := check(req("https://other.example/a"), via); err == nil {
		t.Error("CheckRedirect allowed a cross-host redirect")
	}
	if err := check(req("https://host.example/b"), via); err != nil {
		t.Errorf("CheckRedirect rejected a same-origin path redirect: %v", err)
	}
	long := make([]*http.Request, 10)
	for i := range long {
		long[i] = req("https://host.example/a")
	}
	if err := check(req("https://host.example/z"), long); err == nil {
		t.Error("CheckRedirect allowed more than 10 redirects")
	}
}

func TestHTTPFetch_RejectsUnsafeDest(t *testing.T) {
	fix := newHTTPFixture(t, []byte("x"), `"v1"`)
	src, _ := NewHTTP(HTTPConfig{URL: fix.srv.URL + "/file"})
	if _, err := src.Fetch(context.Background(), "relative/path"); !errors.Is(err, ErrUnsafeDestination) {
		t.Fatalf("Fetch err = %v; want errors.Is(..., ErrUnsafeDestination)", err)
	}
	if got := fix.gets.Load() + fix.heads.Load(); got != 0 {
		t.Fatalf("network was hit %d times before dest validation", got)
	}
}
