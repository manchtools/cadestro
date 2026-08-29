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
