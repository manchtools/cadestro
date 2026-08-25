// Package idp provides identity provider integration for OIDC SSO.
package idp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"syscall"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// OIDCProvider wraps the go-oidc provider and OAuth2 config.
type OIDCProvider struct {
	Provider    *oidc.Provider
	OAuth2Cfg   oauth2.Config
	Verifier    *oidc.IDTokenVerifier
	GroupClaim  string
	UserinfoURL string
	// httpClient bounds every outbound OIDC call (discovery, token exchange,
	// lazy JWKS keyset fetch) with connect/handshake/response timeouts (WS5
	// #6/#14). Without it go-oidc falls back to http.DefaultClient, which has
	// no timeout — a slow/hung IdP (or an attacker-controlled one reached via a
	// public SSOCallback) could hang a request indefinitely. Threaded into
	// every call via oidc.ClientContext.
	httpClient *http.Client
}

// oidcHTTPTimeout is the overall per-request ceiling for outbound OIDC calls.
// A package var (not a const) so tests can shrink it to assert the timeout
// fires without a multi-second wait.
var oidcHTTPTimeout = 12 * time.Second

// ssrfSafeDialControl is a net.Dialer.Control hook (spec 29 S4) that refuses to
// connect to internal addresses. It runs AFTER DNS resolution with the concrete
// IP being dialed, so it defends against SSRF regardless of the configured
// issuer/token URL (including DNS that resolves a public name to a private IP).
// Blocks loopback, RFC1918/ULA private, link-local (incl. 169.254.169.254 cloud
// metadata), and the unspecified address.
// oidcDialControl is the dial-control installed on the OIDC HTTP client. A
// package var (like oidcHTTPTimeout) so the idp test binary can disable it —
// httptest servers listen on loopback, which the guard correctly blocks in
// production; the guard's logic is unit-tested via ssrfSafeDialControl directly.
var oidcDialControl = ssrfSafeDialControl

func ssrfSafeDialControl(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("ssrf guard: cannot parse dial address %q: %w", address, err)
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("ssrf guard: dial address %q is not an IP", host)
	}
	// net.IP.IsPrivate covers RFC1918 + ULA but NOT RFC 6598 shared address space
	// (100.64.0.0/10), reachable as internal services behind CGNAT / in some cloud
	// VPCs — block it explicitly.
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || cgnatNet.Contains(ip) {
		return fmt.Errorf("ssrf guard: refusing to dial internal address %s", ip)
	}
	return nil
}

// cgnatNet is RFC 6598 Shared Address Space (100.64.0.0/10).
var cgnatNet = func() *net.IPNet {
	_, n, err := net.ParseCIDR("100.64.0.0/10")
	if err != nil {
		panic(err)
	}
	return n
}()

// newBoundedOIDCClient returns an *http.Client with connect, TLS-handshake,
// response-header and overall timeouts so no outbound OIDC call can hang, plus an
// SSRF dial-control denylist (spec 29 S4) so a misconfigured/attacker-set
// issuer/token URL cannot make the server probe its internal network.
func newBoundedOIDCClient() *http.Client {
	return &http.Client{
		Timeout: oidcHTTPTimeout,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: 5 * time.Second,
				Control: oidcDialControl,
			}).DialContext,
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: 8 * time.Second,
		},
	}
}

// ProviderConfig holds the configuration needed to create an OIDC provider.
type ProviderConfig struct {
	IssuerURL        string
	AuthorizationURL string
	TokenURL         string
	UserinfoURL      string
	ClientID         string
	ClientSecret     string
	Scopes           []string
	RedirectURL      string
	GroupClaim       string
}

// NewOIDCProvider creates a new OIDC provider by performing discovery.
func NewOIDCProvider(ctx context.Context, cfg ProviderConfig) (*OIDCProvider, error) {
	httpClient := newBoundedOIDCClient()
	// Inject the bounded client BEFORE discovery; go-oidc stores it
	// (getClient(ctx)) and threads it into the lazy keyset fetch the Verifier
	// uses, so the JWKS GET inherits the same timeout.
	ctx = oidc.ClientContext(ctx, httpClient)
	provider, err := oidc.NewProvider(ctx, cfg.IssuerURL)
	if err != nil {
		return nil, fmt.Errorf("oidc discovery: %w", err)
	}

	scopes := cfg.Scopes
	if len(scopes) == 0 {
		scopes = []string{oidc.ScopeOpenID, "profile", "email"}
	}

	endpoint := provider.Endpoint()
	if cfg.AuthorizationURL != "" {
		endpoint.AuthURL = cfg.AuthorizationURL
	}
	if cfg.TokenURL != "" {
		endpoint.TokenURL = cfg.TokenURL
	}

	oauth2Cfg := oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		Endpoint:     endpoint,
		Scopes:       scopes,
		RedirectURL:  cfg.RedirectURL,
	}

	verifier := provider.Verifier(&oidc.Config{
		ClientID: cfg.ClientID,
	})

	userinfoURL := cfg.UserinfoURL
	if userinfoURL == "" {
		// Use the one from discovery
		var claims struct {
			UserinfoEndpoint string `json:"userinfo_endpoint"`
		}
		if err := provider.Claims(&claims); err == nil {
			userinfoURL = claims.UserinfoEndpoint
		}
	}

	return &OIDCProvider{
		Provider:    provider,
		OAuth2Cfg:   oauth2Cfg,
		Verifier:    verifier,
		GroupClaim:  cfg.GroupClaim,
		UserinfoURL: userinfoURL,
		httpClient:  httpClient,
	}, nil
}

// clientCtx threads the bounded HTTP client onto ctx so token exchange and the
// lazy JWKS keyset fetch (during Verify) inherit the connect/response timeouts.
func (p *OIDCProvider) clientCtx(ctx context.Context) context.Context {
	if p.httpClient == nil {
		return ctx
	}
	return oidc.ClientContext(ctx, p.httpClient)
}

// AuthCodeURL generates the authorization URL with PKCE and nonce.
func (p *OIDCProvider) AuthCodeURL(state, nonce, codeVerifier string) string {
	opts := []oauth2.AuthCodeOption{
		oidc.Nonce(nonce),
		oauth2.S256ChallengeOption(codeVerifier),
	}
	return p.OAuth2Cfg.AuthCodeURL(state, opts...)
}

// ExchangeCode exchanges an authorization code for tokens.
func (p *OIDCProvider) ExchangeCode(ctx context.Context, code, codeVerifier string) (*oauth2.Token, error) {
	opts := []oauth2.AuthCodeOption{
		oauth2.VerifierOption(codeVerifier),
	}
	return p.OAuth2Cfg.Exchange(p.clientCtx(ctx), code, opts...)
}

// UserClaims holds the extracted claims from an OIDC id_token or userinfo.
type UserClaims struct {
	Subject           string
	Email             string
	Name              string
	GivenName         string
	FamilyName        string
	PreferredUsername string
	Picture           string
	Locale            string
	Groups            []string
}

type oidcClaims map[string]json.RawMessage

// VerifyAndExtractClaims verifies the id_token and extracts claims.
func (p *OIDCProvider) VerifyAndExtractClaims(ctx context.Context, oauth2Token *oauth2.Token, expectedNonce string) (*UserClaims, error) {
	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		return nil, fmt.Errorf("no id_token in token response")
	}
	return p.VerifyIDToken(ctx, rawIDToken, expectedNonce)
}

// VerifyIDToken verifies a raw OIDC assertion received from a public client.
func (p *OIDCProvider) VerifyIDToken(ctx context.Context, rawIDToken, expectedNonce string) (*UserClaims, error) {
	idToken, err := p.Verifier.Verify(p.clientCtx(ctx), rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("verify id_token: %w", err)
	}

	if idToken.Nonce != expectedNonce {
		return nil, fmt.Errorf("nonce mismatch")
	}

	var claims oidcClaims
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("extract claims: %w", err)
	}

	userClaims := &UserClaims{
		Subject: idToken.Subject,
	}

	if email, ok := claimString(claims["email"]); ok && email != "" {
		if claimIsTrue(claims["email_verified"]) {
			userClaims.Email = email
		} else {
			slog.Warn("SSO: ignoring email claim because email_verified is not true; it will not be used for auto-link or auto-create",
				"subject", idToken.Subject)
		}
	}
	if name, ok := claimString(claims["name"]); ok {
		userClaims.Name = name
	}
	if v, ok := claimString(claims["given_name"]); ok {
		userClaims.GivenName = v
	}
	if v, ok := claimString(claims["family_name"]); ok {
		userClaims.FamilyName = v
	}
	if v, ok := claimString(claims["preferred_username"]); ok {
		userClaims.PreferredUsername = v
	}
	if v, ok := claimString(claims["picture"]); ok {
		userClaims.Picture = v
	}
	if v, ok := claimString(claims["locale"]); ok {
		userClaims.Locale = v
	}

	if p.GroupClaim != "" {
		userClaims.Groups = extractGroups(claims, p.GroupClaim)
	}

	return userClaims, nil
}

func claimString(v json.RawMessage) (string, bool) {
	var value *string
	if err := json.Unmarshal(v, &value); err != nil || value == nil {
		return "", false
	}
	return *value, true
}

func claimIsTrue(v json.RawMessage) bool {
	var boolValue bool
	if err := json.Unmarshal(v, &boolValue); err == nil {
		return boolValue
	}
	var stringValue string
	return json.Unmarshal(v, &stringValue) == nil && stringValue == "true"
}

func extractGroups(claims oidcClaims, claimName string) []string {
	var value json.RawMessage
	parts := strings.Split(claimName, ".")
	current := claims
	for i, part := range parts {
		v, ok := current[part]
		if !ok {
			return nil
		}
		if i == len(parts)-1 {
			value = v
		} else {
			var next oidcClaims
			if err := json.Unmarshal(v, &next); err != nil {
				return nil
			}
			current = next
		}
	}

	if value == nil {
		return nil
	}

	var groupString *string
	if err := json.Unmarshal(value, &groupString); err == nil {
		if groupString == nil {
			return nil
		}
		return strings.Fields(*groupString)
	}

	var rawGroups []json.RawMessage
	if err := json.Unmarshal(value, &rawGroups); err != nil {
		return nil
	}
	groups := make([]string, 0, len(rawGroups))
	for _, rawGroup := range rawGroups {
		if group, ok := claimString(rawGroup); ok {
			groups = append(groups, group)
		}
	}
	return groups
}
