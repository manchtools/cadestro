package remote

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	sysfs "github.com/manchtools/cadestro/sdk/sys/fs"
)

const defaultHTTPMaxBytes int64 = 2 * 1024 * 1024 * 1024

// RedirectPolicy governs which HTTP redirects a fetch follows. The zero value
// (RedirectSameOrigin) keeps the historical default so existing callers are
// unchanged; RedirectNone is stricter (no redirects at all) and
// RedirectCrossOrigin looser (also follows host changes). The constants are
// declared so the default is the zero value, NOT in strictness order. An
// https -> http downgrade is refused at EVERY level (a redirect must never strip
// TLS), and the chain is bounded to 10 hops wherever redirects are followed.
type RedirectPolicy int

const (
	// RedirectSameOrigin (the default / zero value) follows only same-scheme,
	// same-host redirects (path or query changes); a host or scheme change is
	// refused. This pins the bytes to the configured origin for unpinned fetches.
	RedirectSameOrigin RedirectPolicy = iota
	// RedirectNone refuses every redirect — the fetch must reach exactly its URL.
	RedirectNone
	// RedirectCrossOrigin additionally follows host changes and http -> https
	// upgrades (e.g. a CDN such as GitHub releases redirecting github.com ->
	// release-assets.githubusercontent.com). Integrity for cross-origin fetches
	// must come from a ChecksumSHA256 pin, not from host-pinning.
	RedirectCrossOrigin
)

// HTTPConfig configures a public-HTTP Source. Authentication is
// deliberately not modelled — v1 is anonymous-only. v2 adds an Auth
// type without breaking this struct's binary layout.
type HTTPConfig struct {
	// URL of the payload. https:// and http:// schemes only; no
	// userinfo, no fragment, no control characters.
	URL string

	// ChecksumSHA256 — optional, hex-encoded (64 chars). When set, the
	// fetched body is hashed during streaming and the result compared
	// against this value; mismatch is ErrIntegrity. Strongly recommended
	// for any production deploy, mandatory in combination with
	// Extract+Prune (a malicious origin could otherwise poison a
	// destructive sync).
	ChecksumSHA256 string

	// Extract — when true, the payload is treated as an archive
	// (tar.gz / zip / tar.xz, detected by Content-Type + filename)
	// and unpacked into the destination directory. When false, the
	// payload is written verbatim to the destination path. Slice 5
	// introduces this branch; Slice 4 covers the !Extract path.
	Extract bool

	// Prune — for archive payloads only. When true, files present
	// locally in the destination but absent from the archive are
	// removed after a successful extract (mirror-with-delete). For
	// single-file payloads this field is invalid (NewHTTP rejects it).
	Prune bool

	// MaxBytes — hard size cap on the streamed body. Zero means
	// defaultHTTPMaxBytes (2 GiB). The cap is enforced via a one-byte
	// over-read sentinel, so a runaway stream surfaces as ErrIntegrity
	// before the excess hits disk.
	MaxBytes int64

	// Mode / Owner / Group — applied to the destination after a
	// successful Fetch via os.Chmod / sys/fs.FchownNoFollow. Empty
	// strings leave the OS default in place.
	Mode  string
	Owner string
	Group string

	// Client overrides the *http.Client used for the HEAD/GET round-trips.
	// Nil (the default) uses an internal client with conservative timeouts and
	// the cross-origin / scheme-downgrade redirect guard. This is a TRANSPORT
	// seam — its purpose is to point a test at an httptest TLS server (whose
	// self-signed cert the default client refuses) or to mock the round-tripper.
	// It does NOT relax any Fetch invariant: the URL scheme check, the MaxBytes
	// cap, the sha256 pin, and the atomic rename are enforced in Fetch regardless
	// of which client transports the bytes. A supplied client owns its own
	// redirect/transport policy (it does not inherit the default redirect guard).
	Client *http.Client

	// Redirect selects which redirects the default client follows. The zero
	// value (RedirectSameOrigin) preserves the historical strict guard. Set
	// RedirectCrossOrigin for sources behind a CDN that bounces to another host
	// (e.g. GitHub release downloads). Ignored when Client is supplied — a
	// caller-provided client owns its own redirect policy.
	Redirect RedirectPolicy
}

type httpSource struct {
	parsedURL *url.URL
	cfg       HTTPConfig
	checksum  []byte
	client    *http.Client

	mu       sync.Mutex
	revision string
}

// NewHTTP validates cfg and returns a Source. Returns ErrInvalidConfig
// on any validation failure.
func NewHTTP(cfg HTTPConfig) (Source, error) {

	h, err := newHTTPSource(cfg)
	if err != nil {
		return nil, err
	}
	return h, nil
}

func newHTTPSource(cfg HTTPConfig) (*httpSource, error) {

	cfg.URL = strings.TrimSpace(cfg.URL)
	if err := validateHTTPConfig(&cfg); err != nil {
		return nil, err
	}
	parsed, err := parseHTTPURL(cfg.URL)
	if err != nil {
		return nil, err
	}

	var checksum []byte
	if cfg.ChecksumSHA256 != "" {
		b, derr := hex.DecodeString(strings.ToLower(cfg.ChecksumSHA256))
		if derr != nil || len(b) != 32 {
			return nil, fmt.Errorf("%w: checksum_sha256 must be 64 hex chars", ErrInvalidConfig)
		}
		checksum = b
	}

	if cfg.MaxBytes <= 0 {
		cfg.MaxBytes = defaultHTTPMaxBytes
	}

	client := cfg.Client
	if client == nil {
		client = defaultHTTPClient(cfg.Redirect)
	}

	return &httpSource{
		parsedURL: parsed,
		cfg:       cfg,
		checksum:  checksum,
		client:    client,
	}, nil
}

// Fetch downloads the payload to dest. Single-file path only — the
// archive branch lands in Slice 5.
//
// Flow:
//  1. Validate dest. Path safety errors short-circuit before any network
//     round-trip so a misconfigured action can't reveal anything via
//     timing.
//  2. If a previous Revision is known, issue a HEAD with
//     If-None-Match. Origin returns 304 → no-op short-circuit, no GET.
//  3. GET (also with If-None-Match in case the HEAD path was skipped).
//  4. Stream the body to <dest>.tmp.<rand> through a LimitReader (cap
//     +1 to detect overrun) and a sha256.Hash. Cancel + clean up on any
//     mid-stream error.
//  5. Verify the optional sha256 pin.
//  6. os.Rename to dest — gives the atomic-write guarantee.
//  7. Apply mode (and, in real deployments, owner/group via
//     sys/fs.FchownNoFollow when running with privilege).
//  8. RecordDest(dest) so a follow-up Wipe can reach it even when dest
//     lives outside the project-managed prefixes.
func (h *httpSource) Fetch(ctx context.Context, dest string) (Result, error) {
	if h.cfg.Extract {
		return h.fetchArchive(ctx, dest)
	}
	if err := validateDestination(dest); err != nil {
		return Result{}, err
	}

	h.mu.Lock()
	cachedRevision := h.revision
	h.mu.Unlock()

	if cachedRevision != "" {
		notModified, err := h.checkNotModified(ctx, cachedRevision)
		if err != nil {
			return Result{}, err
		}
		if notModified {
			return Result{Changed: false, Revision: cachedRevision}, nil
		}
	}

	body, etag, err := h.openBody(ctx, cachedRevision)
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = body.Close() }()

	tmp, written, sum, err := streamToTmp(dest, body, h.cfg.MaxBytes)
	if err != nil {
		return Result{}, err
	}

	if h.checksum != nil && subtle.ConstantTimeCompare(sum, h.checksum) != 1 {

		_ = os.Remove(tmp)
		return Result{}, fmt.Errorf("%w: sha256 mismatch for %s", ErrIntegrity, dest)
	}

	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return Result{}, fmt.Errorf("rename to %s: %w", dest, err)
	}

	if err := applyMode(dest, h.cfg.Mode, h.cfg.Owner, h.cfg.Group); err != nil {
		return Result{}, err
	}

	revision := etag
	if revision == "" {

		revision = hex.EncodeToString(sum)
	}

	h.mu.Lock()
	h.revision = revision
	h.mu.Unlock()

	RecordDest(dest)

	return Result{
		Changed:      true,
		BytesWritten: written,
		FilesTouched: 1,
		Digest:       hex.EncodeToString(sum),
		Revision:     revision,
	}, nil
}

// Wipe forwards to the shared wipeDest implementation.
func (h *httpSource) Wipe(ctx context.Context, dest string) error {
	return wipeDest(ctx, dest)
}

// String returns a short, human-readable handle used in log lines and
// CommandOutput summaries.
func (h *httpSource) String() string {
	mode := "file"
	if h.cfg.Extract {
		mode = "archive"
	}
	return fmt.Sprintf("http %s [%s]", h.cfg.URL, mode)
}

func (h *httpSource) maxBytes() int64 { return h.cfg.MaxBytes }

func (h *httpSource) checkNotModified(ctx context.Context, etag string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, h.cfg.URL, nil)
	if err != nil {
		return false, fmt.Errorf("HEAD %s: %w", h.cfg.URL, err)
	}
	req.Header.Set("If-None-Match", etag)
	resp, err := h.client.Do(req)
	if err != nil {
		return false, fmt.Errorf("HEAD %s: %w", h.cfg.URL, err)
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusNotModified, nil
}

func (h *httpSource) openBody(ctx context.Context, etag string) (io.ReadCloser, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.cfg.URL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("GET %s: %w", h.cfg.URL, err)
	}
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("GET %s: %w", h.cfg.URL, err)
	}
	if resp.StatusCode == http.StatusNotModified {

		_ = resp.Body.Close()
		return io.NopCloser(strings.NewReader("")), resp.Header.Get("ETag"), nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_ = resp.Body.Close()
		return nil, "", fmt.Errorf("GET %s: status %d", h.cfg.URL, resp.StatusCode)
	}
	return resp.Body, resp.Header.Get("ETag"), nil
}

func streamToTmp(dest string, body io.Reader, maxBytes int64) (string, int64, []byte, error) {
	tmp, err := tmpPathFor(dest)
	if err != nil {
		return "", 0, nil, err
	}
	if err := os.MkdirAll(filepath.Dir(tmp), 0o755); err != nil {
		return "", 0, nil, fmt.Errorf("mkdir %s: %w", filepath.Dir(tmp), err)
	}
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC|os.O_EXCL, 0o600)
	if err != nil {

		if errors.Is(err, os.ErrExist) {
			_ = os.Remove(tmp)
			f, err = os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC|os.O_EXCL, 0o600)
		}
		if err != nil {
			return "", 0, nil, fmt.Errorf("open tmp %s: %w", tmp, err)
		}
	}
	cleanup := func() {
		_ = f.Close()
		_ = os.Remove(tmp)
	}

	limited := io.LimitReader(body, maxBytes+1)
	h := sha256.New()
	tee := io.TeeReader(limited, h)
	n, err := io.Copy(f, tee)
	if err != nil {
		cleanup()
		return "", 0, nil, fmt.Errorf("stream to %s: %w", tmp, err)
	}
	if n > maxBytes {
		cleanup()
		return "", 0, nil, fmt.Errorf("%w: payload exceeds %d bytes", ErrIntegrity, maxBytes)
	}
	if err := f.Sync(); err != nil {
		cleanup()
		return "", 0, nil, fmt.Errorf("fsync %s: %w", tmp, err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return "", 0, nil, fmt.Errorf("close %s: %w", tmp, err)
	}
	return tmp, n, h.Sum(nil), nil
}

func tmpPathFor(dest string) (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate staging-file suffix: %w", err)
	}
	return dest + ".tmp." + hex.EncodeToString(b[:]), nil
}

func applyMode(dest, mode, owner, group string) error {
	if mode == "" && owner == "" && group == "" {
		return nil
	}
	if mode != "" {

		bits, perr := strconv.ParseUint(mode, 8, 32)
		if perr != nil {
			return fmt.Errorf("invalid mode %q: %w", mode, perr)
		}
		if err := os.Chmod(dest, os.FileMode(bits)); err != nil {
			return fmt.Errorf("chmod %s: %w", dest, err)
		}
	}
	if owner != "" || group != "" {
		uid, gid, err := sysfs.ResolveOwnership(owner, group)
		if err != nil {
			return fmt.Errorf("resolve ownership for %s: %w", dest, err)
		}
		if err := chownNoFollow(dest, uid, gid); err != nil {
			return fmt.Errorf("set ownership on %s: %w", dest, err)
		}
	}
	return nil
}

func chownNoFollow(dest string, uid, gid int) error {
	info, err := os.Lstat(dest)
	if err != nil {
		return err
	}
	if info.IsDir() {
		d, err := sysfs.OpenRealDir(dest)
		if err != nil {
			return err
		}
		defer func() { _ = d.Close() }()
		return d.Chown(uid, gid)
	}
	return sysfs.FchownNoFollow(dest, uid, gid)
}

func defaultHTTPClient(p RedirectPolicy) *http.Client {
	return &http.Client{
		Timeout:       30 * time.Minute,
		CheckRedirect: redirectPolicy(p),
	}
}

func redirectPolicy(p RedirectPolicy) func(req *http.Request, via []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		if len(via) == 0 {
			return nil
		}
		from := via[len(via)-1].URL
		to := req.URL

		if from.Scheme == "https" && to.Scheme == "http" {
			return fmt.Errorf("%w: refusing scheme downgrade %s://%s -> %s://%s", ErrInvalidConfig,
				from.Scheme, from.Host, to.Scheme, to.Host)
		}
		switch p {
		case RedirectNone:
			return fmt.Errorf("%w: refusing redirect (policy: none) %s://%s -> %s://%s", ErrInvalidConfig,
				from.Scheme, from.Host, to.Scheme, to.Host)
		case RedirectSameOrigin:
			if to.Scheme != from.Scheme || to.Host != from.Host {
				return fmt.Errorf("%w: refusing cross-origin redirect %s://%s -> %s://%s", ErrInvalidConfig,
					from.Scheme, from.Host, to.Scheme, to.Host)
			}
		case RedirectCrossOrigin:

		default:
			return fmt.Errorf("%w: unknown redirect policy %d", ErrInvalidConfig, p)
		}
		if len(via) >= 10 {
			return fmt.Errorf("%w: stopped after 10 redirects", ErrInvalidConfig)
		}
		return nil
	}
}

func validateHTTPConfig(cfg *HTTPConfig) error {
	if cfg.Prune && !cfg.Extract {
		return fmt.Errorf("%w: prune requires extract", ErrInvalidConfig)
	}

	if cfg.Extract && cfg.Prune && cfg.ChecksumSHA256 == "" {
		return fmt.Errorf("%w: extract+prune requires a checksum_sha256 (the prune deletes files; the payload must be integrity-pinned)", ErrInvalidConfig)
	}

	if cfg.MaxBytes < 0 {
		return fmt.Errorf("%w: max_bytes must not be negative", ErrInvalidConfig)
	}
	if err := validateModeBits(cfg.Mode); err != nil {
		return err
	}
	return nil
}

func validateModeBits(mode string) error {
	if mode == "" {
		return nil
	}

	bits, err := strconv.ParseUint(mode, 8, 32)
	if err != nil {
		return fmt.Errorf("%w: invalid mode %q", ErrInvalidConfig, mode)
	}
	if bits&0o7000 != 0 {
		return fmt.Errorf("%w: mode %q sets privileged bits (setuid/setgid/sticky), refused for a downloaded artifact", ErrInvalidConfig, mode)
	}
	return nil
}

func parseHTTPURL(raw string) (*url.URL, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("%w: url is empty", ErrInvalidConfig)
	}
	if strings.ContainsAny(raw, "\x00\n\r") {
		return nil, fmt.Errorf("%w: url contains control characters", ErrInvalidConfig)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	if !u.IsAbs() {
		return nil, fmt.Errorf("%w: url is not absolute", ErrInvalidConfig)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return nil, fmt.Errorf("%w: scheme %q not supported (https or http only)", ErrInvalidConfig, u.Scheme)
	}
	if u.User != nil {
		return nil, fmt.Errorf("%w: url must not include userinfo", ErrInvalidConfig)
	}
	if u.Fragment != "" {
		return nil, fmt.Errorf("%w: url must not include a fragment", ErrInvalidConfig)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("%w: url has no host", ErrInvalidConfig)
	}
	return u, nil
}
