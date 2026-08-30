package core

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/server/internal/auth"
	"github.com/manchtools/cadestro/server/internal/idp"
	"github.com/manchtools/cadestro/server/internal/store"
	db "github.com/manchtools/cadestro/server/internal/store/generated"
)

func testService(t *testing.T) (*Service, context.Context, time.Time, ed25519.PrivateKey) {
	t.Helper()
	ctx := context.Background()
	database, err := store.New(ctx, filepath.Join(t.TempDir(), "control.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	now := time.Date(2026, time.January, 1, 12, 0, 0, 0, time.UTC)
	manager, err := auth.NewJWTManager(auth.JWTConfig{PrivateKey: privateKey, Now: func() time.Time { return now }})
	require.NoError(t, err)
	return &Service{store: database, jwt: manager, logger: slog.New(slog.NewTextHandler(io.Discard, nil)), now: func() time.Time { return now }}, ctx, now, privateKey
}

func testUser(t *testing.T, service *Service, ctx context.Context, id string) *db.User {
	t.Helper()
	user, err := service.store.Queries().CreateUser(ctx, db.CreateUserParams{ID: id, Email: id + "@example.com", DisplayName: id, CreatedAt: time.Now().UTC(), LastLoginAt: time.Now().UTC()})
	require.NoError(t, err)
	return user
}

func TestRefreshAndLogoutRotateSessionGeneration(t *testing.T) {
	service, ctx, now, privateKey := testService(t)
	user := testUser(t, service, ctx, "01K00000000000000000000001")
	role, err := service.CreateRole(ctx, connect.NewRequest(&cadestrov1.CreateRoleRequest{Name: "Reader", Permissions: []cadestrov1.Permission{cadestrov1.Permission_PERMISSION_GET_CURRENT_USER}}))
	require.NoError(t, err)
	_, err = service.AssignRoleToUser(ctx, connect.NewRequest(&cadestrov1.AssignRoleToUserRequest{UserId: &cadestrov1.UserId{Value: user.ID}, RoleId: role.Msg.Role.Id}))
	require.NoError(t, err)
	user, err = service.store.Queries().GetUser(ctx, user.ID)
	require.NoError(t, err)
	require.EqualValues(t, 2, user.SessionVersion)
	manager, err := auth.NewJWTManager(auth.JWTConfig{PrivateKey: privateKey, Now: func() time.Time { return now }, RefreshTokenExpiry: time.Hour})
	require.NoError(t, err)
	service.jwt = manager
	pair, err := manager.GenerateTokens(user.ID, user.Email, user.SessionVersion, nil)
	require.NoError(t, err)
	refreshed, err := service.RefreshToken(ctx, connect.NewRequest(&cadestrov1.RefreshTokenRequest{RefreshToken: pair.RefreshToken}))
	require.NoError(t, err)
	claims, err := manager.ValidateToken(refreshed.Msg.AccessToken, auth.TokenTypeAccess)
	require.NoError(t, err)
	require.EqualValues(t, 3, claims.SessionVersion)
	require.Equal(t, []cadestrov1.Permission{cadestrov1.Permission_PERMISSION_GET_CURRENT_USER}, claims.Permissions)
	_, err = manager.ValidateToken(refreshed.Msg.RefreshToken, auth.TokenTypeRefresh)
	require.NoError(t, err)
	_, err = service.RefreshToken(ctx, connect.NewRequest(&cadestrov1.RefreshTokenRequest{RefreshToken: pair.RefreshToken}))
	require.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
	user, err = service.store.Queries().GetUser(ctx, user.ID)
	require.NoError(t, err)
	require.EqualValues(t, 3, user.SessionVersion)
	logout, err := service.Logout(ctx, connect.NewRequest(&cadestrov1.LogoutRequest{RefreshToken: refreshed.Msg.RefreshToken}))
	require.NoError(t, err)
	require.NotNil(t, logout)
	user, err = service.store.Queries().GetUser(ctx, user.ID)
	require.NoError(t, err)
	require.EqualValues(t, 4, user.SessionVersion)
	_, err = service.Logout(ctx, connect.NewRequest(&cadestrov1.LogoutRequest{RefreshToken: refreshed.Msg.RefreshToken}))
	require.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
	expiredManager, err := auth.NewJWTManager(auth.JWTConfig{PrivateKey: privateKey, Now: func() time.Time { return now.Add(-2 * time.Hour) }, RefreshTokenExpiry: time.Hour})
	require.NoError(t, err)
	expired, err := expiredManager.GenerateTokens(user.ID, user.Email, user.SessionVersion, nil)
	require.NoError(t, err)
	_, err = service.RefreshToken(ctx, connect.NewRequest(&cadestrov1.RefreshTokenRequest{RefreshToken: expired.RefreshToken}))
	require.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
	_, err = service.Logout(ctx, connect.NewRequest(&cadestrov1.LogoutRequest{RefreshToken: expired.RefreshToken}))
	require.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
	user, err = service.store.Queries().GetUser(ctx, user.ID)
	require.NoError(t, err)
	require.EqualValues(t, 4, user.SessionVersion)
}

func TestRoleMutationsRotateAffectedSessions(t *testing.T) {
	service, ctx, _, _ := testService(t)
	admin := testUser(t, service, ctx, "01K00000000000000000000001")
	target := testUser(t, service, ctx, "01K00000000000000000000002")
	_, err := service.AssignRoleToUser(ctx, connect.NewRequest(&cadestrov1.AssignRoleToUserRequest{UserId: &cadestrov1.UserId{Value: admin.ID}, RoleId: &cadestrov1.RoleId{Value: administratorsRoleID}}))
	require.NoError(t, err)
	custom, err := service.CreateRole(ctx, connect.NewRequest(&cadestrov1.CreateRoleRequest{Name: "Custom", Permissions: []cadestrov1.Permission{cadestrov1.Permission_PERMISSION_LIST_USERS}}))
	require.NoError(t, err)
	_, err = service.AssignRoleToUser(ctx, connect.NewRequest(&cadestrov1.AssignRoleToUserRequest{UserId: &cadestrov1.UserId{Value: target.ID}, RoleId: custom.Msg.Role.Id}))
	require.NoError(t, err)
	require.EqualValues(t, 2, mustUser(t, service, ctx, target.ID).SessionVersion)
	_, err = service.UpdateRole(ctx, connect.NewRequest(&cadestrov1.UpdateRoleRequest{Id: custom.Msg.Role.Id, Name: "Custom", Permissions: []cadestrov1.Permission{cadestrov1.Permission_PERMISSION_GET_CURRENT_USER}}))
	require.NoError(t, err)
	require.EqualValues(t, 3, mustUser(t, service, ctx, target.ID).SessionVersion)
	_, err = service.DeleteRole(ctx, connect.NewRequest(&cadestrov1.DeleteRoleRequest{Id: custom.Msg.Role.Id}))
	require.NoError(t, err)
	require.EqualValues(t, 4, mustUser(t, service, ctx, target.ID).SessionVersion)
	_, err = service.RevokeRoleFromUser(ctx, connect.NewRequest(&cadestrov1.RevokeRoleFromUserRequest{UserId: &cadestrov1.UserId{Value: target.ID}, RoleId: custom.Msg.Role.Id}))
	require.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	version := mustUser(t, service, ctx, admin.ID).SessionVersion
	_, err = service.RevokeRoleFromUser(ctx, connect.NewRequest(&cadestrov1.RevokeRoleFromUserRequest{UserId: &cadestrov1.UserId{Value: admin.ID}, RoleId: &cadestrov1.RoleId{Value: administratorsRoleID}}))
	require.NoError(t, err)
	require.EqualValues(t, version+1, mustUser(t, service, ctx, admin.ID).SessionVersion)
	_, err = service.UpdateRole(ctx, connect.NewRequest(&cadestrov1.UpdateRoleRequest{Id: &cadestrov1.RoleId{Value: administratorsRoleID}, Name: "Administrators"}))
	require.NoError(t, err)
	_, err = service.DeleteRole(ctx, connect.NewRequest(&cadestrov1.DeleteRoleRequest{Id: &cadestrov1.RoleId{Value: administratorsRoleID}}))
	require.NoError(t, err)
	_, err = service.RevokeUserSessions(ctx, connect.NewRequest(&cadestrov1.RevokeUserSessionsRequest{UserId: &cadestrov1.UserId{Value: target.ID}}))
	require.NoError(t, err)
	require.EqualValues(t, 5, mustUser(t, service, ctx, target.ID).SessionVersion)
	_, err = service.RevokeUserSessions(ctx, connect.NewRequest(&cadestrov1.RevokeUserSessionsRequest{UserId: &cadestrov1.UserId{Value: "01K00000000000000000000099"}}))
	require.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

func TestOIDCUsersReceiveSeedRolesByCreationOrder(t *testing.T) {
	service, ctx, now, _ := testService(t)
	_, err := service.store.Queries().CreateIdentityProvider(ctx, db.CreateIdentityProviderParams{ID: "01K00000000000000000000010", Name: "SSO", Slug: "sso", Enabled: true, ClientID: "client", IssuerUrl: "https://issuer.example", ScopesJson: "[]", CreatedAt: now, UpdatedAt: now})
	require.NoError(t, err)
	first, err := service.linkIdentity(ctx, "01K00000000000000000000010", &idp.UserClaims{Subject: "first", Email: "first@example.com", Name: "First"})
	require.NoError(t, err)
	second, err := service.linkIdentity(ctx, "01K00000000000000000000010", &idp.UserClaims{Subject: "second", Email: "second@example.com", Name: "Second"})
	require.NoError(t, err)
	firstRoles, err := service.store.Queries().ListUserRoles(ctx, first.ID)
	require.NoError(t, err)
	secondRoles, err := service.store.Queries().ListUserRoles(ctx, second.ID)
	require.NoError(t, err)
	require.Equal(t, administratorsRoleID, firstRoles[0].ID)
	require.Equal(t, usersRoleID, secondRoles[0].ID)
	updated, err := service.linkIdentity(ctx, "01K00000000000000000000010", &idp.UserClaims{Subject: "first", Email: "first@example.com", Name: "First"})
	require.NoError(t, err)
	require.EqualValues(t, 2, updated.SessionVersion)
}

func TestOIDCUsersSucceedWithoutDeletedSeedRoles(t *testing.T) {
	service, ctx, now, _ := testService(t)
	_, err := service.DeleteRole(ctx, connect.NewRequest(&cadestrov1.DeleteRoleRequest{Id: &cadestrov1.RoleId{Value: administratorsRoleID}}))
	require.NoError(t, err)
	_, err = service.DeleteRole(ctx, connect.NewRequest(&cadestrov1.DeleteRoleRequest{Id: &cadestrov1.RoleId{Value: usersRoleID}}))
	require.NoError(t, err)
	_, err = service.store.Queries().CreateIdentityProvider(ctx, db.CreateIdentityProviderParams{ID: "01K00000000000000000000010", Name: "SSO", Slug: "sso", Enabled: true, ClientID: "client", IssuerUrl: "https://issuer.example", ScopesJson: "[]", CreatedAt: now, UpdatedAt: now})
	require.NoError(t, err)
	first, err := service.linkIdentity(ctx, "01K00000000000000000000010", &idp.UserClaims{Subject: "first", Email: "first@example.com", Name: "First"})
	require.NoError(t, err)
	second, err := service.linkIdentity(ctx, "01K00000000000000000000010", &idp.UserClaims{Subject: "second", Email: "second@example.com", Name: "Second"})
	require.NoError(t, err)
	firstRoles, err := service.store.Queries().ListUserRoles(ctx, first.ID)
	require.NoError(t, err)
	secondRoles, err := service.store.Queries().ListUserRoles(ctx, second.ID)
	require.NoError(t, err)
	require.Empty(t, firstRoles)
	require.Empty(t, secondRoles)
}

func mustUser(t *testing.T, service *Service, ctx context.Context, id string) *db.User {
	t.Helper()
	user, err := service.store.Queries().GetUser(ctx, id)
	require.NoError(t, err)
	return user
}
