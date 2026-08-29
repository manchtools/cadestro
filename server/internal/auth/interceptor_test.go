package auth

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/contract/gen/go/cadestro/v1/cadestrov1connect"
	"github.com/stretchr/testify/require"
)

func TestAccessTokenCarriesSessionVersion(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	manager, err := NewJWTManager(JWTConfig{PrivateKey: privateKey})
	require.NoError(t, err)
	pair, err := manager.GenerateTokens("01K00000000000000000000001", "admin@example.com", 4, nil)
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
	interceptor := NewInterceptor(manager)
	request := connect.NewRequest(&struct{}{})
	_, err = interceptor.WrapUnary(func(context.Context, connect.AnyRequest) (connect.AnyResponse, error) { return nil, nil })(context.Background(), request)
	require.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
}

func TestPermissionGateRequiresMappedPermission(t *testing.T) {
	required, ok := PermissionForProcedure(cadestrov1connect.ControlServiceListUsersProcedure)
	require.True(t, ok)
	require.Equal(t, cadestrov1.Permission_PERMISSION_LIST_USERS, required)
	require.True(t, userHasPermission([]cadestrov1.Permission{required}, required))
	require.False(t, userHasPermission([]cadestrov1.Permission{cadestrov1.Permission_PERMISSION_GET_CURRENT_USER}, required))
}

type permissionTestHandler struct {
	cadestrov1connect.UnimplementedControlServiceHandler
	called bool
}

func (handler *permissionTestHandler) ListUsers(context.Context, *connect.Request[cadestrov1.ListUsersRequest]) (*connect.Response[cadestrov1.ListUsersResponse], error) {
	handler.called = true
	return connect.NewResponse(&cadestrov1.ListUsersResponse{}), nil
}

type bearerTransport struct {
	token string
}

func (transport bearerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	request.Header.Set("Authorization", "Bearer "+transport.token)
	return http.DefaultTransport.RoundTrip(request)
}

func TestInterceptorPermitsAndDeniesMappedRPCByClaim(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	manager, err := NewJWTManager(JWTConfig{PrivateKey: privateKey})
	require.NoError(t, err)
	handler := &permissionTestHandler{}
	path, httpHandler := cadestrov1connect.NewControlServiceHandler(handler, connect.WithInterceptors(connect.UnaryInterceptorFunc(NewInterceptor(manager).WrapUnary)))
	mux := http.NewServeMux()
	mux.Handle(path, httpHandler)
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	allowed, err := manager.GenerateTokens("01K00000000000000000000001", "admin@example.com", 1, []cadestrov1.Permission{cadestrov1.Permission_PERMISSION_LIST_USERS})
	require.NoError(t, err)
	allowedClient := cadestrov1connect.NewControlServiceClient(&http.Client{Transport: bearerTransport{token: allowed.AccessToken}}, server.URL)
	_, err = allowedClient.ListUsers(context.Background(), connect.NewRequest(&cadestrov1.ListUsersRequest{}))
	require.NoError(t, err)
	require.True(t, handler.called)
	denied, err := manager.GenerateTokens("01K00000000000000000000001", "admin@example.com", 1, nil)
	require.NoError(t, err)
	deniedClient := cadestrov1connect.NewControlServiceClient(&http.Client{Transport: bearerTransport{token: denied.AccessToken}}, server.URL)
	_, err = deniedClient.ListUsers(context.Background(), connect.NewRequest(&cadestrov1.ListUsersRequest{}))
	require.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
}
