package remote

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

type s3PrefixFixture struct {
	srv     *httptest.Server
	bucket  string
	prefix  string
	objects []s3TestObject
	listErr int
	getErr  int
	lists   atomic.Int32
	gets    atomic.Int32
}

type s3TestObject struct {
	key  string
	body []byte
	etag string
}

func newS3PrefixFixture(t *testing.T, bucket, prefix string, objs []s3TestObject) *s3PrefixFixture {
	t.Helper()
	f := &s3PrefixFixture{bucket: bucket, prefix: prefix, objects: objs}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		if r.URL.Path == "/"+f.bucket || r.URL.Path == "/"+f.bucket+"/" {
			if r.URL.Query().Get("list-type") != "2" {
				http.Error(w, "expected list-type=2", http.StatusBadRequest)
				return
			}
			f.lists.Add(1)
			if f.listErr != 0 {
				w.WriteHeader(f.listErr)
				return
			}
			writeListResponse(w, f.bucket, f.prefix, f.objects)
			return
		}

		key := strings.TrimPrefix(r.URL.Path, "/"+f.bucket+"/")
		for _, o := range f.objects {
			if o.key == key {
				w.Header().Set("ETag", o.etag)
				if r.Method == http.MethodHead {
					w.WriteHeader(http.StatusOK)
					return
				}
				if f.getErr != 0 {
					w.WriteHeader(f.getErr)
					return
				}
				f.gets.Add(1)
				_, _ = w.Write(o.body)
				return
			}
		}
		http.Error(w, "no such key", http.StatusNotFound)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func writeListResponse(w http.ResponseWriter, bucket, prefix string, objs []s3TestObject) {
	type entry struct {
		Key  string `xml:"Key"`
		ETag string `xml:"ETag"`
		Size int    `xml:"Size"`
	}
	type result struct {
		XMLName     xml.Name `xml:"ListBucketResult"`
		Name        string   `xml:"Name"`
		Prefix      string   `xml:"Prefix"`
		KeyCount    int      `xml:"KeyCount"`
		IsTruncated bool     `xml:"IsTruncated"`
		Contents    []entry  `xml:"Contents"`
	}
	res := result{Name: bucket, Prefix: prefix, KeyCount: len(objs)}
	for _, o := range objs {
		res.Contents = append(res.Contents, entry{Key: o.key, ETag: o.etag, Size: len(o.body)})
	}
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	_ = xml.NewEncoder(w).Encode(res)
	_, _ = fmt.Fprintln(w)
}

func TestS3Fetch_PrefixSync_MirrorsTree(t *testing.T) {
	objs := []s3TestObject{
		{key: "data/a.txt", body: []byte("alpha"), etag: `"a1"`},
		{key: "data/sub/b.txt", body: []byte("bravo"), etag: `"b1"`},
		{key: "data/sub/c.txt", body: []byte("charlie"), etag: `"c1"`},
	}
	fix := newS3PrefixFixture(t, "mybucket", "data/", objs)
	dest := filepath.Join(t.TempDir(), "tree")
	recordDestUnder(t, dest)

	src, err := NewS3(S3Config{
		Endpoint: fix.srv.URL,
		Bucket:   "mybucket",
		Key:      "data/",
	})
	if err != nil {
		t.Fatalf("NewS3: %v", err)
	}
	res, err := src.Fetch(context.Background(), dest)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !res.Changed || res.FilesTouched != 3 {
		t.Fatalf("Result = %+v; want Changed=true FilesTouched=3", res)
	}
	assertFile(t, filepath.Join(dest, "a.txt"), "alpha")
	assertFile(t, filepath.Join(dest, "sub", "b.txt"), "bravo")
	assertFile(t, filepath.Join(dest, "sub", "c.txt"), "charlie")
}

func TestS3Fetch_PrefixSync_NoOpOnSameList(t *testing.T) {
	objs := []s3TestObject{
		{key: "p/a", body: []byte("x"), etag: `"a"`},
	}
	fix := newS3PrefixFixture(t, "b", "p/", objs)
	dest := filepath.Join(t.TempDir(), "tree")
	recordDestUnder(t, dest)

	src, _ := NewS3(S3Config{Endpoint: fix.srv.URL, Bucket: "b", Key: "p/"})
	if _, err := src.Fetch(context.Background(), dest); err != nil {
		t.Fatalf("Fetch #1: %v", err)
	}
	gets1 := fix.gets.Load()
	res2, err := src.Fetch(context.Background(), dest)
	if err != nil {
		t.Fatalf("Fetch #2: %v", err)
	}
	if res2.Changed {
		t.Fatal("second Fetch Changed=true on unchanged listing")
	}
	if fix.gets.Load() != gets1 {
		t.Fatalf("second Fetch issued object GETs: %d → %d", gets1, fix.gets.Load())
	}
}

func TestS3Fetch_PrefixSync_PruneRemovesLocalExtras(t *testing.T) {
	for _, prune := range []bool{false, true} {
		t.Run(fmt.Sprintf("Prune=%v", prune), func(t *testing.T) {
			objs := []s3TestObject{{key: "p/a", body: []byte("x"), etag: `"a"`}}
			fix := newS3PrefixFixture(t, "b", "p/", objs)
			dest := filepath.Join(t.TempDir(), "tree")
			recordDestUnder(t, dest)

			src, _ := NewS3(S3Config{
				Endpoint: fix.srv.URL,
				Bucket:   "b",
				Key:      "p/",
				Prune:    prune,
			})
			if _, err := src.Fetch(context.Background(), dest); err != nil {
				t.Fatalf("first Fetch: %v", err)
			}

			extra := filepath.Join(dest, "untracked")
			if err := os.WriteFile(extra, []byte("local"), 0o600); err != nil {
				t.Fatalf("write extra: %v", err)
			}

			fix.objects = append(fix.objects, s3TestObject{key: "p/b", body: []byte("y"), etag: `"b"`})
			if _, err := src.Fetch(context.Background(), dest); err != nil {
				t.Fatalf("second Fetch: %v", err)
			}

			_, statErr := os.Stat(extra)
			if prune && !os.IsNotExist(statErr) {
				t.Fatalf("Prune=true but extra survived: %v", statErr)
			}
			if !prune && statErr != nil {
				t.Fatalf("Prune=false but extra removed: %v", statErr)
			}
		})
	}
}

func TestS3Fetch_PrefixSync_PathPrefixedEndpoint(t *testing.T) {
	const bucket = "mybucket"
	objs := []s3TestObject{{key: "data/a.txt", body: []byte("alpha"), etag: `"a1"`}}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/"+bucket &&
			r.URL.Query().Get("list-type") == "2" && r.URL.Query().Get("prefix") == "data/":
			writeListResponse(w, bucket, "data/", objs)
		case r.Method == http.MethodHead && r.URL.Path == "/v1/"+bucket+"/data/a.txt":
			w.Header().Set("ETag", `"a1"`)
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/"+bucket+"/data/a.txt":
			w.Header().Set("ETag", `"a1"`)
			_, _ = w.Write([]byte("alpha"))
		default:
			http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	dest := filepath.Join(t.TempDir(), "tree")
	recordDestUnder(t, dest)

	src, err := NewS3(S3Config{
		Endpoint: srv.URL + "/v1",
		Bucket:   bucket,
		Key:      "data/",
	})
	if err != nil {
		t.Fatalf("NewS3: %v", err)
	}
	res, err := src.Fetch(context.Background(), dest)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !res.Changed || res.FilesTouched != 1 {
		t.Fatalf("Result = %+v; want Changed=true FilesTouched=1", res)
	}
	assertFile(t, filepath.Join(dest, "a.txt"), "alpha")
}

func TestS3Fetch_PrefixSync_403OnList_SurfacesClearError(t *testing.T) {
	fix := newS3PrefixFixture(t, "b", "p/", nil)
	fix.listErr = http.StatusForbidden
	dest := filepath.Join(t.TempDir(), "tree")
	recordDestUnder(t, dest)
	src, _ := NewS3(S3Config{Endpoint: fix.srv.URL, Bucket: "b", Key: "p/"})

	_, err := src.Fetch(context.Background(), dest)
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("Fetch err = %v; want ErrInvalidConfig", err)
	}
}
