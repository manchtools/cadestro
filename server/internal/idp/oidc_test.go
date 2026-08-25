package idp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manchtools/cadestro/server/internal/idp"
)

func fakeOIDCServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                srv.URL,
			"authorization_endpoint":                srv.URL + "/authorize",
			"token_endpoint":                        srv.URL + "/token",
			"userinfo_endpoint":                     srv.URL + "/userinfo",
			"jwks_uri":                              srv.URL + "/jwks.json",
			"response_types_supported":              []string{"code"},
			"subject_types_supported":               []string{"public"},
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})

	mux.HandleFunc("/jwks.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{}})
	})

	return srv
}

func TestNewOIDCProvider_DiscoverySucceeds(t *testing.T) {
	srv := fakeOIDCServer(t)

	p, err := idp.NewOIDCProvider(context.Background(), idp.ProviderConfig{
		IssuerURL:    srv.URL,
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		RedirectURL:  "https://app.example.com/auth/callback/test",
	})
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.NotNil(t, p.Provider, "discovery must populate the underlying go-oidc Provider")
	assert.NotNil(t, p.Verifier, "Verifier must be constructed for later id_token validation")
}

func TestNewOIDCProvider_DiscoveryFailsOnUnreachableIssuer(t *testing.T) {

	_, err := idp.NewOIDCProvider(context.Background(), idp.ProviderConfig{
		IssuerURL: "http://127.0.0.1:1",
		ClientID:  "test",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "oidc discovery",
		"discovery failure must be wrapped with the 'oidc discovery' prefix so the SSO handler can surface it")
}

func TestNewOIDCProvider_DefaultsScopesWhenEmpty(t *testing.T) {

	srv := fakeOIDCServer(t)
	p, err := idp.NewOIDCProvider(context.Background(), idp.ProviderConfig{
		IssuerURL:   srv.URL,
		ClientID:    "test-client",
		RedirectURL: "https://app.example.com/cb",
	})
	require.NoError(t, err)

	authURL := p.AuthCodeURL("st", "nc", "verifier-with-enough-entropy-yes-quite")
	parsed, err := url.Parse(authURL)
	require.NoError(t, err)
	scope := parsed.Query().Get("scope")
	for _, want := range []string{"openid", "profile", "email"} {
		assert.Contains(t, scope, want, "default scopes must include %q", want)
	}
}

func TestOIDCProvider_AuthCodeURL_EmitsStateNoncePKCE(t *testing.T) {
	srv := fakeOIDCServer(t)
	p, err := idp.NewOIDCProvider(context.Background(), idp.ProviderConfig{
		IssuerURL:   srv.URL,
		ClientID:    "test-client",
		RedirectURL: "https://app.example.com/cb",
		Scopes:      []string{"openid"},
	})
	require.NoError(t, err)

	authURL := p.AuthCodeURL("the-state-token", "the-nonce", "code-verifier-of-sufficient-length")
	parsed, err := url.Parse(authURL)
	require.NoError(t, err)
	q := parsed.Query()

	assert.Equal(t, "the-state-token", q.Get("state"),
		"state MUST round-trip verbatim — SSOCallback's anti-CSRF check compares it against the auth_state row")
	assert.Equal(t, "the-nonce", q.Get("nonce"),
		"nonce MUST round-trip verbatim — id_token verification compares it against this value")
	assert.NotEmpty(t, q.Get("code_challenge"),
		"PKCE code_challenge MUST be present — the auth code exchange depends on it")
	assert.Equal(t, "S256", q.Get("code_challenge_method"),
		"PKCE method must be S256, not plain")
}

func TestOIDCProvider_AuthCodeURL_HonoursAuthorizationURLOverride(t *testing.T) {

	srv := fakeOIDCServer(t)
	p, err := idp.NewOIDCProvider(context.Background(), idp.ProviderConfig{
		IssuerURL:        srv.URL,
		AuthorizationURL: "https://override.example.com/authorize",
		ClientID:         "test",
		RedirectURL:      "https://app.example.com/cb",
	})
	require.NoError(t, err)

	authURL := p.AuthCodeURL("s", "n", "v-of-enough-length-please")
	assert.True(t, strings.HasPrefix(authURL, "https://override.example.com/authorize"),
		"AuthorizationURL override must take precedence over the discovered endpoint; got %s", authURL)
}
