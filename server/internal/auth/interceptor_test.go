package auth

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
)

func TestAccessTokenCarriesSessionVersion(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	manager, err := NewJWTManager(JWTConfig{PrivateKey: privateKey})
	require.NoError(t, err)
	pair, err := manager.GenerateTokens("01K00000000000000000000001", "admin@example.com", 4)
	require.NoError(t, err)
	claims, err := manager.ValidateToken(pair.AccessToken, TokenTypeAccess)
	require.NoError(t, err)
	require.EqualValues(t, 4, claims.SessionVersion)
	_, err = manager.ValidateToken(pair.RefreshToken, TokenTypeAccess)
	require.Error(t, err)
}

func TestInterceptorRejectsMissingBearer(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	manager, err := NewJWTManager(JWTConfig{PrivateKey: privateKey})
	require.NoError(t, err)
	interceptor := NewInterceptor(manager, func(context.Context, string) (string, int32, error) { return "", 0, nil })
	request := connect.NewRequest(&struct{}{})
	_, err = interceptor.WrapUnary(func(context.Context, connect.AnyRequest) (connect.AnyResponse, error) { return nil, nil })(context.Background(), request)
	require.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
}
