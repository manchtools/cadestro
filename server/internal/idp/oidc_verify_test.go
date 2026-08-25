package idp_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"github.com/manchtools/cadestro/server/internal/idp"
)

type signedOIDCFixture struct {
	srv  *httptest.Server
	priv *rsa.PrivateKey
	kid  string
}

func newSignedOIDCFixture(t *testing.T) *signedOIDCFixture {
	t.Helper()

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	kid := "test-key-1"

	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                srv.URL,
			"authorization_endpoint":                srv.URL + "/authorize",
			"token_endpoint":                        srv.URL + "/token",
			"jwks_uri":                              srv.URL + "/jwks.json",
			"response_types_supported":              []string{"code"},
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})

	mux.HandleFunc("/jwks.json", func(w http.ResponseWriter, r *http.Request) {
		jwks := jose.JSONWebKeySet{
			Keys: []jose.JSONWebKey{{
				Key:       &priv.PublicKey,
				KeyID:     kid,
				Algorithm: "RS256",
				Use:       "sig",
			}},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jwks)
	})

	return &signedOIDCFixture{srv: srv, priv: priv, kid: kid}
}

func (f *signedOIDCFixture) signIDToken(t *testing.T, audience, nonce string, extra map[string]any) string {
	t.Helper()

	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.RS256, Key: f.priv},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", f.kid),
	)
	require.NoError(t, err)

	now := time.Now()
	claims := map[string]any{
		"iss":   f.srv.URL,
		"sub":   "test-subject-123",
		"aud":   audience,
		"exp":   now.Add(5 * time.Minute).Unix(),
		"iat":   now.Unix(),
		"nonce": nonce,
	}
	for k, v := range extra {
		claims[k] = v
	}

	raw, err := jwt.Signed(signer).Claims(claims).Serialize()
	require.NoError(t, err)
	return raw
}

func TestVerifyAndExtractClaims_HappyPath(t *testing.T) {
	f := newSignedOIDCFixture(t)
	p, err := idp.NewOIDCProvider(context.Background(), idp.ProviderConfig{
		IssuerURL:   f.srv.URL,
		ClientID:    "test-client",
		RedirectURL: "https://app.example.com/cb",
		GroupClaim:  "realm_access.roles",
	})
	require.NoError(t, err)

	idToken := f.signIDToken(t, "test-client", "the-expected-nonce", map[string]any{
		"email":          "alice@example.com",
		"email_verified": true,
		"name":           "Alice Test",
		"given_name":     "Alice",
		"realm_access":   map[string]any{"roles": []string{"admin", "operator"}},
	})
	tok := (&oauth2.Token{}).WithExtra(map[string]any{"id_token": idToken})

	claims, err := p.VerifyAndExtractClaims(context.Background(), tok, "the-expected-nonce")
	require.NoError(t, err)
	assert.Equal(t, "test-subject-123", claims.Subject)
	assert.Equal(t, "alice@example.com", claims.Email)
	assert.Equal(t, "Alice Test", claims.Name)
	assert.Equal(t, "Alice", claims.GivenName)
	assert.Equal(t, []string{"admin", "operator"}, claims.Groups)
}

func TestVerifyAndExtractClaims_EmailVerifiedGate(t *testing.T) {
	cases := []struct {
		name            string
		email           string
		emailPresent    bool
		emailVerified   any
		verifiedPresent bool
		wantEmail       string
	}{
		{name: "verified (bool true)", email: "alice@example.com", emailPresent: true, emailVerified: true, verifiedPresent: true, wantEmail: "alice@example.com"},
		{name: "verified (string \"true\")", email: "alice@example.com", emailPresent: true, emailVerified: "true", verifiedPresent: true, wantEmail: "alice@example.com"},
		{name: "unverified (bool false)", email: "alice@example.com", emailPresent: true, emailVerified: false, verifiedPresent: true, wantEmail: ""},
		{name: "unverified (string \"false\")", email: "alice@example.com", emailPresent: true, emailVerified: "false", verifiedPresent: true, wantEmail: ""},
		{name: "email_verified claim absent", email: "alice@example.com", emailPresent: true, verifiedPresent: false, wantEmail: ""},
		{name: "email claim absent (verified true)", emailPresent: false, emailVerified: true, verifiedPresent: true, wantEmail: ""},
		{name: "email empty string (verified true)", email: "", emailPresent: true, emailVerified: true, verifiedPresent: true, wantEmail: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newSignedOIDCFixture(t)
			p, err := idp.NewOIDCProvider(context.Background(), idp.ProviderConfig{
				IssuerURL:   f.srv.URL,
				ClientID:    "test-client",
				RedirectURL: "https://app.example.com/cb",
			})
			require.NoError(t, err)

			extra := map[string]any{}
			if tc.emailPresent {
				extra["email"] = tc.email
			}
			if tc.verifiedPresent {
				extra["email_verified"] = tc.emailVerified
			}
			idToken := f.signIDToken(t, "test-client", "n", extra)
			tok := (&oauth2.Token{}).WithExtra(map[string]any{"id_token": idToken})

			claims, err := p.VerifyAndExtractClaims(context.Background(), tok, "n")
			require.NoError(t, err)
			assert.Equal(t, "test-subject-123", claims.Subject, "subject is always populated")
			assert.Equalf(t, tc.wantEmail, claims.Email,
				"email must be populated only when email_verified is true")
		})
	}
}

func TestVerifyAndExtractClaims_NonceMismatch(t *testing.T) {

	f := newSignedOIDCFixture(t)
	p, err := idp.NewOIDCProvider(context.Background(), idp.ProviderConfig{
		IssuerURL:   f.srv.URL,
		ClientID:    "test-client",
		RedirectURL: "https://app.example.com/cb",
	})
	require.NoError(t, err)

	idToken := f.signIDToken(t, "test-client", "captured-nonce-from-other-session", nil)
	tok := (&oauth2.Token{}).WithExtra(map[string]any{"id_token": idToken})

	_, err = p.VerifyAndExtractClaims(context.Background(), tok, "the-expected-nonce")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nonce mismatch",
		"nonce mismatch must surface with the explicit 'nonce mismatch' message — operators rely on this string for anti-replay diagnostics")
}

func TestVerifyAndExtractClaims_NoIDToken(t *testing.T) {

	f := newSignedOIDCFixture(t)
	p, err := idp.NewOIDCProvider(context.Background(), idp.ProviderConfig{
		IssuerURL:   f.srv.URL,
		ClientID:    "test-client",
		RedirectURL: "https://app.example.com/cb",
	})
	require.NoError(t, err)

	tokWithoutID := &oauth2.Token{}
	_, err = p.VerifyAndExtractClaims(context.Background(), tokWithoutID, "any")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no id_token in token response")
}

func TestVerifyAndExtractClaims_InvalidSignature(t *testing.T) {

	f := newSignedOIDCFixture(t)
	p, err := idp.NewOIDCProvider(context.Background(), idp.ProviderConfig{
		IssuerURL:   f.srv.URL,
		ClientID:    "test-client",
		RedirectURL: "https://app.example.com/cb",
	})
	require.NoError(t, err)

	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.RS256, Key: otherKey},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", "rogue-key"),
	)
	require.NoError(t, err)
	now := time.Now()
	raw, err := jwt.Signed(signer).Claims(map[string]any{
		"iss":   f.srv.URL,
		"sub":   "test-subject",
		"aud":   "test-client",
		"exp":   now.Add(5 * time.Minute).Unix(),
		"iat":   now.Unix(),
		"nonce": "n",
	}).Serialize()
	require.NoError(t, err)

	tok := (&oauth2.Token{}).WithExtra(map[string]any{"id_token": raw})
	_, err = p.VerifyAndExtractClaims(context.Background(), tok, "n")
	require.Error(t, err, "id_token signed by a non-JWKS key MUST fail verification")
	assert.Contains(t, err.Error(), "verify id_token")
}

func verifyProviderForFixture(t *testing.T, f *signedOIDCFixture) *idp.OIDCProvider {
	t.Helper()
	p, err := idp.NewOIDCProvider(context.Background(), idp.ProviderConfig{
		IssuerURL:   f.srv.URL,
		ClientID:    "test-client",
		RedirectURL: "https://app.example.com/cb",
	})
	require.NoError(t, err)
	return p
}

func TestVerifyAndExtractClaims_RejectsWrongAudience(t *testing.T) {
	f := newSignedOIDCFixture(t)
	p := verifyProviderForFixture(t, f)
	idToken := f.signIDToken(t, "some-other-client", "n", map[string]any{"email_verified": true})
	tok := (&oauth2.Token{}).WithExtra(map[string]any{"id_token": idToken})
	_, err := p.VerifyAndExtractClaims(context.Background(), tok, "n")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "verify id_token")
}

func TestVerifyAndExtractClaims_RejectsExpired(t *testing.T) {
	f := newSignedOIDCFixture(t)
	p := verifyProviderForFixture(t, f)
	idToken := f.signIDToken(t, "test-client", "n", map[string]any{
		"exp": time.Now().Add(-1 * time.Hour).Unix(),
	})
	tok := (&oauth2.Token{}).WithExtra(map[string]any{"id_token": idToken})
	_, err := p.VerifyAndExtractClaims(context.Background(), tok, "n")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "verify id_token")
}

func TestVerifyAndExtractClaims_RejectsWrongIssuer(t *testing.T) {
	f := newSignedOIDCFixture(t)
	p := verifyProviderForFixture(t, f)
	idToken := f.signIDToken(t, "test-client", "n", map[string]any{
		"iss": "https://evil.example",
	})
	tok := (&oauth2.Token{}).WithExtra(map[string]any{"id_token": idToken})
	_, err := p.VerifyAndExtractClaims(context.Background(), tok, "n")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "verify id_token")
}

func TestVerifyAndExtractClaims_ByteTamperedSignature(t *testing.T) {
	f := newSignedOIDCFixture(t)
	p := verifyProviderForFixture(t, f)
	idToken := f.signIDToken(t, "test-client", "n", map[string]any{"email_verified": true})

	parts := strings.Split(idToken, ".")
	require.Len(t, parts, 3)
	sig := []byte(parts[2])
	require.Greater(t, len(sig), 2)
	idx := len(sig) / 2
	if sig[idx] == 'A' {
		sig[idx] = 'B'
	} else {
		sig[idx] = 'A'
	}
	parts[2] = string(sig)
	tampered := strings.Join(parts, ".")

	tok := (&oauth2.Token{}).WithExtra(map[string]any{"id_token": tampered})
	_, err := p.VerifyAndExtractClaims(context.Background(), tok, "n")
	require.Error(t, err, "a byte-tampered signature MUST fail verification")
	assert.Contains(t, err.Error(), "verify id_token")
}
