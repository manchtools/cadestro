package identity_test

import (
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/server/internal/auth"
)

func TestApiToken_CreateListAuthenticateAndRevoke(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	actor := f.seedActor(grant{Permissions: allPermissionKeys()})
	scopeID := newULID()
	f.insertUserRoleGrant(actor.ID, f.insertRole([]string{"ListDevices:assigned"}), auth.ScopeKindDeviceGroup, scopeID)
	expiresAt := timestamppb.New(f.now.Add(time.Hour))
	created, err := f.client.CreateApiToken(f.ctx(), authed(&cadestrov1.CreateApiTokenRequest{Name: "automation", ExpiresAt: expiresAt}, actor.Token))
	require.NoError(t, err)
	require.NotEmpty(t, created.Msg.Value)
	claims, err := f.jwt.ValidateToken(created.Msg.Value, auth.TokenTypeAPIToken)
	require.NoError(t, err)
	assert.Equal(t, actor.ID, claims.UserID)
	assert.Equal(t, actor.ID, claims.Subject)
	assert.Equal(t, actor.ID, claims.UserID)
	assert.Contains(t, claims.Permissions, "GetCurrentUser")
	assert.Equal(t, int32(0), claims.SessionVersion)
	foundScope := false
	for _, grant := range claims.ScopedGrants {
		if grant.ScopeID == scopeID {
			foundScope = true
		}
	}
	assert.True(t, foundScope)

	listed, err := f.client.ListApiTokens(f.ctx(), authed(&cadestrov1.ListApiTokensRequest{}, actor.Token))
	require.NoError(t, err)
	require.Len(t, listed.Msg.Tokens, 1)
	assert.Equal(t, created.Msg.Token.Id.Value, listed.Msg.Tokens[0].Id.Value)

	_, err = f.client.GetCurrentUser(f.ctx(), authed(&cadestrov1.GetCurrentUserRequest{}, created.Msg.Value))
	require.NoError(t, err)

	other := f.seedActor(grant{Permissions: allPermissionKeys()})
	_, err = f.client.RevokeApiToken(f.ctx(), authed(&cadestrov1.RevokeApiTokenRequest{Id: created.Msg.Token.Id}, other.Token))
	assert.Equal(t, connect.CodeNotFound, connectCodeOf(t, err))
	_, err = f.client.GetCurrentUser(f.ctx(), authed(&cadestrov1.GetCurrentUserRequest{}, created.Msg.Value))
	require.NoError(t, err)

	_, err = f.client.RevokeApiToken(f.ctx(), authed(&cadestrov1.RevokeApiTokenRequest{Id: created.Msg.Token.Id}, actor.Token))
	require.NoError(t, err)
	_, err = f.client.GetCurrentUser(f.ctx(), authed(&cadestrov1.GetCurrentUserRequest{}, created.Msg.Value))
	assert.Equal(t, connect.CodeUnauthenticated, connectCodeOf(t, err))
}

func TestApiToken_ExpiryAndDisabledUserInvalidateAuthentication(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	admin := f.seedActor(grant{Permissions: allPermissionKeys()})
	created, err := f.client.CreateApiToken(f.ctx(), authed(&cadestrov1.CreateApiTokenRequest{Name: "short", ExpiresAt: timestamppb.New(f.now.Add(time.Minute))}, admin.Token))
	require.NoError(t, err)
	f.advance(2 * time.Minute)
	_, err = f.client.GetCurrentUser(f.ctx(), authed(&cadestrov1.GetCurrentUserRequest{}, created.Msg.Value))
	assert.Equal(t, connect.CodeUnauthenticated, connectCodeOf(t, err))

	active := f.seedActor(grant{Permissions: allPermissionKeys()})
	created, err = f.client.CreateApiToken(f.ctx(), authed(&cadestrov1.CreateApiTokenRequest{Name: "disable", ExpiresAt: timestamppb.New(f.now.Add(time.Hour))}, active.Token))
	require.NoError(t, err)
	_, err = f.client.SetUserDisabled(f.ctx(), authed(&cadestrov1.SetUserDisabledRequest{Id: &cadestrov1.UserId{Value: active.ID}, Disabled: true}, admin.Token))
	require.NoError(t, err)
	_, err = f.client.GetCurrentUser(f.ctx(), authed(&cadestrov1.GetCurrentUserRequest{}, created.Msg.Value))
	assert.Equal(t, connect.CodeUnauthenticated, connectCodeOf(t, err))
}

func TestApiToken_OutlivesAccessTokenAndBrowserLogout(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	actor := f.seedActor(grant{Permissions: allPermissionKeys()})
	pair := f.mintPair(actor.ID, actor.Email)
	created, err := f.client.CreateApiToken(f.ctx(), authed(&cadestrov1.CreateApiTokenRequest{Name: "long", ExpiresAt: timestamppb.New(f.now.Add(time.Hour))}, actor.Token))
	require.NoError(t, err)
	f.advance(f.jwt.AccessTokenTTL() + time.Second)
	_, err = f.client.GetCurrentUser(f.ctx(), authed(&cadestrov1.GetCurrentUserRequest{}, created.Msg.Value))
	require.NoError(t, err)
	_, err = f.client.Logout(f.ctx(), connect.NewRequest(&cadestrov1.LogoutRequest{RefreshToken: pair.RefreshToken}))
	require.NoError(t, err)
	_, err = f.client.GetCurrentUser(f.ctx(), authed(&cadestrov1.GetCurrentUserRequest{}, created.Msg.Value))
	require.NoError(t, err)
}

func TestApiToken_AuthorityChangesInvalidateAuthentication(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	admin := f.seedActor(grant{Permissions: allPermissionKeys()})
	actor := f.seedActor(grant{Permissions: allPermissionKeys()})
	created, err := f.client.CreateApiToken(f.ctx(), authed(&cadestrov1.CreateApiTokenRequest{Name: "authority", ExpiresAt: timestamppb.New(f.now.Add(time.Hour))}, actor.Token))
	require.NoError(t, err)
	_, err = f.raw.Exec(f.ctx(), `UPDATE users SET session_version = session_version + 1 WHERE id = $1`, actor.ID)
	require.NoError(t, err)
	_, err = f.client.GetCurrentUser(f.ctx(), authed(&cadestrov1.GetCurrentUserRequest{}, created.Msg.Value))
	assert.Equal(t, connect.CodeUnauthenticated, connectCodeOf(t, err))

	created, err = f.client.CreateApiToken(f.ctx(), authed(&cadestrov1.CreateApiTokenRequest{Name: "deleted", ExpiresAt: timestamppb.New(f.now.Add(time.Hour))}, admin.Token))
	require.NoError(t, err)
	_, err = f.raw.Exec(f.ctx(), `UPDATE users SET is_deleted = true WHERE id = $1`, admin.ID)
	require.NoError(t, err)
	_, err = f.client.GetCurrentUser(f.ctx(), authed(&cadestrov1.GetCurrentUserRequest{}, created.Msg.Value))
	assert.Equal(t, connect.CodeUnauthenticated, connectCodeOf(t, err))
}

func TestApiToken_StaleAccessCannotMintAfterAuthorityInvalidation(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	actor := f.seedActor(grant{Permissions: allPermissionKeys()})
	_, err := f.raw.Exec(f.ctx(), `UPDATE users SET disabled = true, session_version = session_version + 1 WHERE id = $1`, actor.ID)
	require.NoError(t, err)
	_, err = f.client.CreateApiToken(f.ctx(), authed(&cadestrov1.CreateApiTokenRequest{Name: "stale", ExpiresAt: timestamppb.New(f.now.Add(time.Hour))}, actor.Token))
	assert.Equal(t, connect.CodeUnauthenticated, connectCodeOf(t, err))
}

func TestApiToken_RejectsPastExpiry(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	actor := f.seedActor(grant{Permissions: allPermissionKeys()})
	_, err := f.client.CreateApiToken(f.ctx(), authed(&cadestrov1.CreateApiTokenRequest{Name: "past", ExpiresAt: timestamppb.New(f.now.Add(-time.Minute))}, actor.Token))
	assert.Equal(t, connect.CodeInvalidArgument, connectCodeOf(t, err))
}
