package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/server/internal/idp"
	generated "github.com/manchtools/cadestro/server/internal/store/generated"
	"github.com/stretchr/testify/require"
)

func TestNewAppliesCoreSchema(t *testing.T) {
	store, err := New(context.Background(), filepath.Join(t.TempDir(), "control.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	count, err := store.Queries().CountDevices(context.Background())
	require.NoError(t, err)
	require.Zero(t, count)
}

func TestDatabaseOwnsLifecycleTimestamps(t *testing.T) {
	ctx := context.Background()
	database, err := New(ctx, filepath.Join(t.TempDir(), "control.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	role, err := database.Queries().CreateRole(ctx, generated.CreateRoleParams{ID: "01K00000000000000000000001", Name: "Timestamp test", Description: ""})
	require.NoError(t, err)
	require.WithinDuration(t, time.Now().UTC(), role.CreatedAt, 10*time.Second)
	require.Equal(t, role.CreatedAt, role.UpdatedAt)
	time.Sleep(1100 * time.Millisecond)
	role, err = database.Queries().SetRoleDescription(ctx, generated.SetRoleDescriptionParams{ID: role.ID, Description: role.Description})
	require.NoError(t, err)
	require.True(t, role.UpdatedAt.After(role.CreatedAt))
}

func TestIsConflictOnlyMatchesUniqueAndPrimaryKeyConstraints(t *testing.T) {
	ctx := context.Background()
	database, err := New(ctx, filepath.Join(t.TempDir(), "control.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	role, err := database.Queries().CreateRole(ctx, generated.CreateRoleParams{ID: "01K00000000000000000000011", Name: "Conflict", Description: ""})
	require.NoError(t, err)
	_, err = database.Queries().CreateRole(ctx, generated.CreateRoleParams{ID: "01K00000000000000000000012", Name: role.Name, Description: ""})
	require.Error(t, err)
	require.True(t, IsConflict(err))
	_, err = database.Queries().CreateRole(ctx, generated.CreateRoleParams{ID: role.ID, Name: "Other", Description: ""})
	require.Error(t, err)
	require.True(t, IsConflict(err))
	_, err = database.Queries().GrantRolePermission(ctx, generated.GrantRolePermissionParams{RoleID: role.ID, Permission: cadestrov1.Permission_PERMISSION_UNSPECIFIED})
	require.Error(t, err)
	require.False(t, IsConflict(err))
	_, err = database.Queries().CreateAssignment(ctx, generated.CreateAssignmentParams{ID: "01K00000000000000000000013", ActionID: "01K00000000000000000000099", TargetType: cadestrov1.AssignmentTargetType_ASSIGNMENT_TARGET_TYPE_DEVICE, TargetID: "01K00000000000000000000098"})
	require.Error(t, err)
	require.False(t, IsConflict(err))
}

func TestIdentityProviderScopesRoundTrip(t *testing.T) {
	ctx := context.Background()
	database, err := New(ctx, filepath.Join(t.TempDir(), "control.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	want := idp.Scopes{"openid", "profile", "email"}
	_, err = database.Queries().CreateIdentityProvider(ctx, generated.CreateIdentityProviderParams{
		ID: "01K00000000000000000000014", Name: "SSO", Slug: "sso", Enabled: true,
		ClientID: "client", IssuerUrl: "https://issuer.example", ScopesJson: want,
	})
	require.NoError(t, err)
	provider, err := database.Queries().GetIdentityProvider(ctx, "01K00000000000000000000014")
	require.NoError(t, err)
	require.Equal(t, want, provider.ScopesJson)
}
