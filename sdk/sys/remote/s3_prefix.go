package remote

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func (s *s3Source) fetchPrefix(ctx context.Context, dest string) (Result, error) {
	if err := validateDestination(dest); err != nil {
		return Result{}, err
	}

	objects, err := s.listObjects(ctx)
	if err != nil {
		return Result{}, err
	}
	listHash := hashListing(objects)

	s.mu.Lock()
	cachedRevision := s.revision
	s.mu.Unlock()

	if cachedRevision == listHash {
		return Result{Changed: false, Revision: listHash}, nil
	}

	if err := os.MkdirAll(dest, 0o755); err != nil {
		return Result{}, fmt.Errorf("mkdir %s: %w", dest, err)
	}

	var (
		bytesWritten int64
		filesTouched int
	)
	maxBytes := int64(defaultHTTPMaxBytes)
	relPaths := make(map[string]struct{}, len(objects))

	for _, obj := range objects {
		rel, err := relPathForKey(s.cfg.Key, obj.Key)
		if err != nil {
			return Result{}, err
		}
		relPaths[rel] = struct{}{}

		outPath := filepath.Join(dest, filepath.FromSlash(rel))

		if err := assertWithinDest(dest, outPath); err != nil {
			return Result{}, err
		}

		body, etag, err := s.openSingleObject(ctx, obj.Key)
		if err != nil {
			return Result{}, err
		}
		written, _, err := streamObjectToFile(body, outPath, maxBytes)
		_ = body.Close()
		if err != nil {
			return Result{}, err
		}
		_ = etag
		bytesWritten += written
		filesTouched++
	}

	if s.cfg.Prune {
		if err := pruneTo(dest, relPaths); err != nil {
			return Result{}, fmt.Errorf("prune %s: %w", dest, err)
		}
	}

	if err := applyMode(dest, s.cfg.Mode, s.cfg.Owner, s.cfg.Group); err != nil {
		return Result{}, err
	}

	s.mu.Lock()
	s.revision = listHash
	s.mu.Unlock()
	RecordDest(dest)

	return Result{
		Changed:      true,
		BytesWritten: bytesWritten,
		FilesTouched: filesTouched,
		Revision:     listHash,
	}, nil
}

type s3Object struct {
	Key  string
	ETag string
}

const maxPrefixObjects = 10000

func (s *s3Source) listObjects(ctx context.Context) ([]s3Object, error) {

	bucketURL, err := s.bucketRootURL()
	if err != nil {
		return nil, err
	}

	var all []s3Object
	var token string
	for {
		q := url.Values{}
		q.Set("list-type", "2")
		q.Set("prefix", s.cfg.Key)
		if token != "" {
			q.Set("continuation-token", token)
		}
		bucketURL.RawQuery = q.Encode()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, bucketURL.String(), nil)
		if err != nil {
			return nil, fmt.Errorf("list %s: %w", bucketURL.String(), err)
		}
		resp, err := s.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("list %s: %w", bucketURL.String(), err)
		}
		switch {
		case resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized:
			_ = resp.Body.Close()
			return nil, fmt.Errorf("%w: anonymous list on %s returned %d (bucket policy may need adjustment)", ErrInvalidConfig, bucketURL.String(), resp.StatusCode)
		case resp.StatusCode < 200 || resp.StatusCode >= 300:
			_ = resp.Body.Close()
			return nil, fmt.Errorf("list %s: status %d", bucketURL.String(), resp.StatusCode)
		}

		var parsed listV2Response
		if err := xml.NewDecoder(resp.Body).Decode(&parsed); err != nil {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("decode list %s: %w", bucketURL.String(), err)
		}
		_ = resp.Body.Close()
		for _, e := range parsed.Contents {
			all = append(all, s3Object{Key: e.Key, ETag: e.ETag})
			if len(all) > maxPrefixObjects {
				return nil, fmt.Errorf("%w: prefix %q under %s/%s contains more than %d objects; narrow the prefix or paginate the source", ErrInvalidConfig, s.cfg.Key, s.cfg.Endpoint, s.cfg.Bucket, maxPrefixObjects)
			}
		}
		if !parsed.IsTruncated || parsed.NextContinuationToken == "" {
			break
		}
		token = parsed.NextContinuationToken
	}
	return all, nil
}

type listV2Response struct {
	XMLName               xml.Name `xml:"ListBucketResult"`
	IsTruncated           bool     `xml:"IsTruncated"`
	NextContinuationToken string   `xml:"NextContinuationToken"`
	Contents              []struct {
		Key  string `xml:"Key"`
		ETag string `xml:"ETag"`
	} `xml:"Contents"`
}

func hashListing(objs []s3Object) string {
	sort.Slice(objs, func(i, j int) bool { return objs[i].Key < objs[j].Key })
	h := sha256.New()
	for _, o := range objs {
		_, _ = io.WriteString(h, o.Key)
		_, _ = io.WriteString(h, "\x00")
		_, _ = io.WriteString(h, o.ETag)
		_, _ = io.WriteString(h, "\x00")
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (s *s3Source) bucketRootURL() (*url.URL, error) {
	endpoint, err := parseS3Endpoint(s.cfg.Endpoint)
	if err != nil {
		return nil, err
	}
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/" + s.cfg.Bucket
	return endpoint, nil
}

func (s *s3Source) openSingleObject(ctx context.Context, key string) (io.ReadCloser, string, error) {
	bucketAndKey, err := s.bucketRootURL()
	if err != nil {
		return nil, "", err
	}
	bucketAndKey.Path = bucketAndKey.Path + "/" + key

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, bucketAndKey.String(), nil)
	if err != nil {
		return nil, "", fmt.Errorf("GET %s: %w", bucketAndKey.String(), err)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("GET %s: %w", bucketAndKey.String(), err)
	}
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		_ = resp.Body.Close()
		return nil, "", fmt.Errorf("%w: anonymous GET on %s returned %d", ErrInvalidConfig, bucketAndKey.String(), resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_ = resp.Body.Close()
		return nil, "", fmt.Errorf("GET %s: status %d", bucketAndKey.String(), resp.StatusCode)
	}
	return resp.Body, resp.Header.Get("ETag"), nil
}

func streamObjectToFile(body io.Reader, outPath string, maxBytes int64) (int64, []byte, error) {
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return 0, nil, fmt.Errorf("mkdir %s: %w", filepath.Dir(outPath), err)
	}
	tmp, written, sum, err := streamToTmp(outPath, body, maxBytes)
	if err != nil {
		return 0, nil, err
	}
	if err := os.Rename(tmp, outPath); err != nil {
		_ = os.Remove(tmp)
		return 0, nil, fmt.Errorf("rename to %s: %w", outPath, err)
	}
	return written, sum, nil
}

func relPathForKey(prefix, key string) (string, error) {
	if !strings.HasPrefix(key, prefix) {
		return "", fmt.Errorf("%w: object key %q does not begin with prefix %q", ErrInvalidConfig, key, prefix)
	}
	rel := strings.TrimPrefix(key, prefix)
	if rel == "" {
		return "", fmt.Errorf("%w: object key %q matches the prefix exactly (cannot map to file)", ErrInvalidConfig, key)
	}
	if strings.ContainsRune(rel, 0) {
		return "", fmt.Errorf("%w: object key %q contains NUL", ErrUnsafeDestination, key)
	}
	for _, comp := range strings.Split(rel, "/") {
		if comp == ".." {
			return "", fmt.Errorf("%w: object key %q contains a traversal component", ErrUnsafeDestination, key)
		}
	}
	return rel, nil
}

func assertWithinDest(dest, full string) error {
	destAbs, err := filepath.Abs(dest)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnsafeDestination, err)
	}
	fullAbs, err := filepath.Abs(full)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnsafeDestination, err)
	}
	if !strings.HasPrefix(fullAbs, destAbs+string(filepath.Separator)) && fullAbs != destAbs {
		return fmt.Errorf("%w: %s resolves outside %s", ErrUnsafeDestination, full, dest)
	}
	return nil
}
