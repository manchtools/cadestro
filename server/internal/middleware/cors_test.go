package middleware

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func corsTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
}

func TestCORS_AllowAll_NoCredentialedWildcard(t *testing.T) {
	h := CORS(nil, true, corsTestLogger())(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Origin", "https://evil.example")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	assert.Equal(t, "https://evil.example", w.Header().Get("Access-Control-Allow-Origin"),
		"allow-all reflects the origin")
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Credentials"),
		"allow-all must NOT set Allow-Credentials (credentialed-wildcard hole)")
}

func TestCORS_ExplicitOrigin_KeepsCredentials(t *testing.T) {
	h := CORS([]string{"https://app.example"}, false, corsTestLogger())(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Origin", "https://app.example")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	assert.Equal(t, "https://app.example", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"),
		"a named origin keeps credentials")
}

func TestCORS_AllowHeadersExcludesCookie(t *testing.T) {
	h := CORS([]string{"https://app.example"}, false, corsTestLogger())(okHandler())

	req := httptest.NewRequest(http.MethodOptions, "/x", nil)
	req.Header.Set("Origin", "https://app.example")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code, "allowed-origin preflight is 204")
	allow := splitCSV(w.Header().Get("Access-Control-Allow-Headers"))
	assert.Contains(t, allow, "Authorization", "Bearer auth header must be allowed")
	assert.Contains(t, allow, "Content-Type")
	assert.NotContains(t, allow, "Cookie", "Cookie must NOT be an allowed request header (Bearer-only auth)")
}

func TestCORS_AllowedOriginPreflight204(t *testing.T) {
	h := CORS([]string{"https://app.example"}, false, corsTestLogger())(okHandler())

	req := httptest.NewRequest(http.MethodOptions, "/x", nil)
	req.Header.Set("Origin", "https://app.example")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.NotEmpty(t, w.Header().Get("Access-Control-Allow-Methods"))
	assert.NotEmpty(t, w.Header().Get("Access-Control-Allow-Headers"))
	assert.Equal(t, "86400", w.Header().Get("Access-Control-Max-Age"))
	assert.Contains(t, w.Header().Values("Vary"), "Origin")
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func TestCORS_UnlistedOrigin_NoHeaders(t *testing.T) {
	h := CORS([]string{"https://app.example"}, false, corsTestLogger())(okHandler())

	req := httptest.NewRequest(http.MethodOptions, "/x", nil)
	req.Header.Set("Origin", "https://evil.example")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Credentials"))
}
