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

type OIDCProvider struct {
	Provider   *oidc.Provider
	OAuth2Cfg  oauth2.Config
	Verifier   *oidc.IDTokenVerifier
	GroupClaim string

	httpClient *http.Client
}

var oidcHTTPTimeout = 12 * time.Second

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

	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || cgnatNet.Contains(ip) {
		return fmt.Errorf("ssrf guard: refusing to dial internal address %s", ip)
	}
	return nil
}

var cgnatNet = func() *net.IPNet {
	_, n, err := net.ParseCIDR("100.64.0.0/10")
	if err != nil {
		panic(err)
	}
	return n
}()

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

type ProviderConfig struct {
	IssuerURL        string
	AuthorizationURL string
	TokenURL         string
	ClientID         string
	Scopes           []string
	RedirectURL      string
	GroupClaim       string
}

func NewOIDCProvider(ctx context.Context, cfg ProviderConfig) (*OIDCProvider, error) {
	httpClient := newBoundedOIDCClient()

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
	endpoint.AuthStyle = oauth2.AuthStyleInParams

	oauth2Cfg := oauth2.Config{
		ClientID:    cfg.ClientID,
		Endpoint:    endpoint,
		Scopes:      scopes,
		RedirectURL: cfg.RedirectURL,
	}

	verifier := provider.Verifier(&oidc.Config{
		ClientID: cfg.ClientID,
	})

	return &OIDCProvider{
		Provider:   provider,
		OAuth2Cfg:  oauth2Cfg,
		Verifier:   verifier,
		GroupClaim: cfg.GroupClaim,
		httpClient: httpClient,
	}, nil
}

func (p *OIDCProvider) clientCtx(ctx context.Context) context.Context {
	if p.httpClient == nil {
		return ctx
	}
	return oidc.ClientContext(ctx, p.httpClient)
}

func (p *OIDCProvider) AuthCodeURL(state, nonce, codeVerifier string) string {
	opts := []oauth2.AuthCodeOption{
		oidc.Nonce(nonce),
		oauth2.S256ChallengeOption(codeVerifier),
	}
	return p.OAuth2Cfg.AuthCodeURL(state, opts...)
}

func (p *OIDCProvider) ExchangeCode(ctx context.Context, code, codeVerifier string) (*oauth2.Token, error) {
	opts := []oauth2.AuthCodeOption{
		oauth2.VerifierOption(codeVerifier),
	}
	return p.OAuth2Cfg.Exchange(p.clientCtx(ctx), code, opts...)
}

type UserClaims struct {
	Subject           string
	Email             string
	Name              string
	GivenName         string
	FamilyName        string
	PreferredUsername string
	Locale            string
	Groups            []string
}

type oidcClaims map[string]json.RawMessage

func (p *OIDCProvider) VerifyAndExtractClaims(ctx context.Context, oauth2Token *oauth2.Token, expectedNonce string) (*UserClaims, error) {
	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		return nil, fmt.Errorf("no id_token in token response")
	}
	return p.VerifyIDToken(ctx, rawIDToken, expectedNonce)
}

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
