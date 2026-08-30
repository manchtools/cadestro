package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

func TestGenerateTokensPlacesPermissionsOnlyOnAccessToken(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewJWTManager(JWTConfig{PrivateKey: privateKey})
	if err != nil {
		t.Fatal(err)
	}
	wanted := []cadestrov1.Permission{cadestrov1.Permission_PERMISSION_GET_CURRENT_USER}
	pair, err := manager.GenerateTokens("01K00000000000000000000001", "admin@example.com", 4, wanted)
	if err != nil {
		t.Fatal(err)
	}
	access, err := manager.ValidateToken(pair.AccessToken, TokenTypeAccess)
	if err != nil {
		t.Fatal(err)
	}
	if len(access.Permissions) != 1 || access.Permissions[0] != wanted[0] {
		t.Fatalf("access permissions = %v", access.Permissions)
	}
	refresh, err := manager.ValidateToken(pair.RefreshToken, TokenTypeRefresh)
	if err != nil {
		t.Fatal(err)
	}
	if len(refresh.Permissions) != 0 {
		t.Fatalf("refresh permissions = %v", refresh.Permissions)
	}
}

func TestExpiredAccessTokenIsRejected(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.January, 1, 12, 0, 0, 0, time.UTC)
	manager, err := NewJWTManager(JWTConfig{PrivateKey: privateKey, AccessTokenExpiry: time.Minute, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	pair, err := manager.GenerateTokens("01K00000000000000000000001", "admin@example.com", 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Minute)
	if _, err := manager.ValidateToken(pair.AccessToken, TokenTypeAccess); err == nil {
		t.Fatal("expected expired access token to be rejected")
	}
}

func TestOIDCTransactionRoundTripAndValidation(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.January, 1, 12, 0, 0, 0, time.UTC)
	manager, err := NewJWTManager(JWTConfig{PrivateKey: privateKey, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"state", "provider", "nonce", "verifier", "https://web.example/callback"}
	value, err := manager.SignOIDCTransaction(want[0], want[1], want[2], want[3], want[4], now.Add(10*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	claims, err := manager.ValidateOIDCTransaction(value)
	if err != nil {
		t.Fatal(err)
	}
	if claims.State != want[0] || claims.ProviderID != want[1] || claims.Nonce != want[2] || claims.CodeVerifier != want[3] || claims.RedirectURL != want[4] {
		t.Fatalf("claims = %+v", claims)
	}
	value += "x"
	if _, err := manager.ValidateOIDCTransaction(value); err == nil {
		t.Fatal("expected tampered transaction to be rejected")
	}
}

func TestOIDCTransactionRejectsExpiryAndRequiredFields(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.January, 1, 12, 0, 0, 0, time.UTC)
	manager, err := NewJWTManager(JWTConfig{PrivateKey: privateKey, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	for _, fields := range [][]string{{"", "provider", "nonce", "verifier", "redirect"}, {"state", "", "nonce", "verifier", "redirect"}, {"state", "provider", "", "verifier", "redirect"}, {"state", "provider", "nonce", "", "redirect"}, {"state", "provider", "nonce", "verifier", ""}} {
		if _, err := manager.SignOIDCTransaction(fields[0], fields[1], fields[2], fields[3], fields[4], now.Add(time.Minute)); err == nil {
			t.Fatalf("expected required claim rejection for %v", fields)
		}
	}
	expired, err := manager.SignOIDCTransaction("state", "provider", "nonce", "verifier", "redirect", now.Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.ValidateOIDCTransaction(expired); err == nil {
		t.Fatal("expected expired transaction to be rejected")
	}
}
