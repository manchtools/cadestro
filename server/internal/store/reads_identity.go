package store

import (
	"context"
	"fmt"
	"time"

	"github.com/manchtools/cadestro/server/internal/store/generated"
	"github.com/manchtools/cadestro/server/internal/store/sqlitetype"
)

type (
	RoleRow = generated.Role

	RoleGrantRow = generated.ListUserRoleGrantsRow

	GroupRoleGrantRow = generated.ListUserGroupRoleGrantsRow

	UserGroupView = generated.GetUserGroupViewRow

	UserGroupMemberView = generated.ListUserGroupMembersRow

	UserDynamicEvaluationRow = generated.ListUsersForDynamicUserGroupEvaluationRow

	InheritedRoleRow = generated.ListInheritedRolesForUserRow

	ScopedGrantRow = generated.ListUserScopedGrantsRow

	IdentityProviderRow = generated.IdentityProvider

	IdentityLinkRow = generated.IdentityLink

	IdentityLinkWithProviderRow = generated.ListIdentityLinksForUserRow

	UserSSHKeyRow = generated.UserSshKey

	ServerSettingsRow = generated.ServerSetting

	UserSessionStateRow = generated.GetUserSessionStateRow
)

type UserGroupListFilter struct {
	AfterID         string
	Limit           int32
	ScopeRestricted bool
	ScopeGroupIDs   []string
}

func (s *Store) GetUserByEmail(ctx context.Context, email string) (UserRow, error) {
	row, err := s.queries.GetUserByEmail(ctx, email)
	if err != nil {
		return UserRow{}, fmt.Errorf("user: get by email: %w", translateNotFound(err))
	}
	return row, nil
}

func (s *Store) GetUserSessionState(ctx context.Context, id string) (UserSessionStateRow, error) {
	row, err := s.queries.GetUserSessionState(ctx, id)
	if err != nil {
		return UserSessionStateRow{}, fmt.Errorf("user: session state: %w", translateNotFound(err))
	}
	return row, nil
}

func (s *Store) ListUsers(ctx context.Context, after string, limit int32) ([]UserRow, error) {
	rows, err := s.queries.ListUsers(ctx, generated.ListUsersParams{ID: after, Limit: int64(limit)})
	if err != nil {
		return nil, fmt.Errorf("user: list: %w", err)
	}
	return rows, nil
}

func (s *Store) ListUserPermissions(ctx context.Context, userID string) ([]string, error) {
	rows, err := s.queries.ListUserPermissions(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("user: permissions: %w", err)
	}
	return rows, nil
}

func (s *Store) ListUserScopedGrants(ctx context.Context, userID string) ([]ScopedGrantRow, error) {
	rows, err := s.queries.ListUserScopedGrants(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("user: scoped grants: %w", err)
	}
	return rows, nil
}

func (s *Store) ListUserRoleGrants(ctx context.Context, userID string) ([]RoleGrantRow, error) {
	rows, err := s.queries.ListUserRoleGrants(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("user: role grants: %w", err)
	}
	return rows, nil
}

func (s *Store) ListUserGroupRoleGrants(ctx context.Context, groupID string) ([]GroupRoleGrantRow, error) {
	rows, err := s.queries.ListUserGroupRoleGrants(ctx, groupID)
	if err != nil {
		return nil, fmt.Errorf("user_group: role grants: %w", err)
	}
	return rows, nil
}

func (s *Store) GetUserGroupView(ctx context.Context, id string) (UserGroupView, error) {
	row, err := s.queries.GetUserGroupView(ctx, id)
	if err != nil {
		return UserGroupView{}, fmt.Errorf("user_group: get: %w", translateNotFound(err))
	}
	return row, nil
}

func (s *Store) ListUserGroups(ctx context.Context, filter UserGroupListFilter) ([]UserGroupView, error) {
	if filter.Limit <= 0 || filter.Limit > 101 {
		return nil, fmt.Errorf("user_group: list limit must be between 1 and 101")
	}
	rows, err := s.queries.ListUserGroups(ctx, generated.ListUserGroupsParams{
		AfterID: filter.AfterID, RowLimit: int64(filter.Limit),
		ScopeRestricted: filter.ScopeRestricted, ScopeGroupIdsJson: sqlitetype.StringList(filter.ScopeGroupIDs),
	})
	if err != nil {
		return nil, fmt.Errorf("user_group: list: %w", err)
	}
	groups := make([]UserGroupView, len(rows))
	for i, row := range rows {
		groups[i] = UserGroupView(row)
	}
	return groups, nil
}

func (s *Store) CountUserGroups(ctx context.Context, filter UserGroupListFilter) (int64, error) {
	count, err := s.queries.CountUserGroups(ctx, generated.CountUserGroupsParams{
		ScopeRestricted: filter.ScopeRestricted, ScopeGroupIdsJson: sqlitetype.StringList(filter.ScopeGroupIDs),
	})
	if err != nil {
		return 0, fmt.Errorf("user_group: count: %w", err)
	}
	return count, nil
}

func (s *Store) ListUserGroupsForUser(ctx context.Context, userID string, filter UserGroupListFilter) ([]UserGroupView, error) {
	rows, err := s.queries.ListUserGroupsForUser(ctx, generated.ListUserGroupsForUserParams{
		UserID: userID, ScopeRestricted: filter.ScopeRestricted, ScopeGroupIdsJson: sqlitetype.StringList(filter.ScopeGroupIDs),
	})
	if err != nil {
		return nil, fmt.Errorf("user_group: list for user: %w", err)
	}
	groups := make([]UserGroupView, len(rows))
	for i, row := range rows {
		groups[i] = UserGroupView(row)
	}
	return groups, nil
}

func (s *Store) ListUserGroupMembers(ctx context.Context, groupID string) ([]UserGroupMemberView, error) {
	rows, err := s.queries.ListUserGroupMembers(ctx, groupID)
	if err != nil {
		return nil, fmt.Errorf("user_group: members: %w", err)
	}
	return rows, nil
}

func (s *Store) ListUsersForDynamicUserGroupEvaluation(ctx context.Context) ([]UserDynamicEvaluationRow, error) {
	rows, err := s.queries.ListUsersForDynamicUserGroupEvaluation(ctx)
	if err != nil {
		return nil, fmt.Errorf("user_group: list evaluation users: %w", err)
	}
	return rows, nil
}

func (s *Store) ListInheritedRolesForUser(ctx context.Context, userID string) ([]InheritedRoleRow, error) {
	rows, err := s.queries.ListInheritedRolesForUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("user: inherited roles: %w", err)
	}
	return rows, nil
}

func (s *Store) ListUserGroupIDsForUser(ctx context.Context, userID string) ([]string, error) {
	rows, err := s.queries.ListUserGroupIDsForUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("user: group memberships: %w", err)
	}
	return rows, nil
}

func (s *Store) ListUserSSHKeys(ctx context.Context, userID string) ([]UserSSHKeyRow, error) {
	rows, err := s.queries.ListUserSshKeys(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("user: ssh keys: %w", err)
	}
	return rows, nil
}

func (s *Store) GetRole(ctx context.Context, id string) (RoleRow, error) {
	row, err := s.queries.GetRole(ctx, id)
	if err != nil {
		return RoleRow{}, fmt.Errorf("role: get: %w", translateNotFound(err))
	}
	return row, nil
}

func (s *Store) GetRoleByName(ctx context.Context, name string) (RoleRow, error) {
	row, err := s.queries.GetRoleByName(ctx, name)
	if err != nil {
		return RoleRow{}, fmt.Errorf("role: get by name: %w", translateNotFound(err))
	}
	return row, nil
}

func (s *Store) ListRoles(ctx context.Context, after string, limit int32) ([]RoleRow, error) {
	rows, err := s.queries.ListRoles(ctx, generated.ListRolesParams{ID: after, Limit: int64(limit)})
	if err != nil {
		return nil, fmt.Errorf("role: list: %w", err)
	}
	return rows, nil
}

func (s *Store) CountRoles(ctx context.Context) (int64, error) {
	n, err := s.queries.CountRoles(ctx)
	if err != nil {
		return 0, fmt.Errorf("role: count: %w", err)
	}
	return n, nil
}

func (s *Store) CountRoleHolders(ctx context.Context, roleID string) (int64, error) {
	n, err := s.queries.CountRoleHolders(ctx, roleID)
	if err != nil {
		return 0, fmt.Errorf("role: count holders: %w", err)
	}
	return n, nil
}

func (s *Store) GetIdentityProvider(ctx context.Context, id string) (IdentityProviderRow, error) {
	row, err := s.queries.GetIdentityProvider(ctx, id)
	if err != nil {
		return IdentityProviderRow{}, fmt.Errorf("identity_provider: get: %w", translateNotFound(err))
	}
	return row, nil
}

func (s *Store) GetIdentityProviderBySlug(ctx context.Context, slug string) (IdentityProviderRow, error) {
	row, err := s.queries.GetIdentityProviderBySlug(ctx, slug)
	if err != nil {
		return IdentityProviderRow{}, fmt.Errorf("identity_provider: get by slug: %w", translateNotFound(err))
	}
	return row, nil
}

func (s *Store) ListIdentityProviders(ctx context.Context, after string, limit int32) ([]IdentityProviderRow, error) {
	rows, err := s.queries.ListIdentityProviders(ctx, generated.ListIdentityProvidersParams{ID: after, Limit: int64(limit)})
	if err != nil {
		return nil, fmt.Errorf("identity_provider: list: %w", err)
	}
	return rows, nil
}

func (s *Store) ListEnabledIdentityProviders(ctx context.Context) ([]IdentityProviderRow, error) {
	rows, err := s.queries.ListEnabledIdentityProviders(ctx)
	if err != nil {
		return nil, fmt.Errorf("identity_provider: list enabled: %w", err)
	}
	return rows, nil
}

func (s *Store) CountIdentityProviders(ctx context.Context) (int64, error) {
	n, err := s.queries.CountIdentityProviders(ctx)
	if err != nil {
		return 0, fmt.Errorf("identity_provider: count: %w", err)
	}
	return n, nil
}

func (s *Store) GetIdentityLink(ctx context.Context, id string) (IdentityLinkRow, error) {
	row, err := s.queries.GetIdentityLink(ctx, id)
	if err != nil {
		return IdentityLinkRow{}, fmt.Errorf("identity_link: get: %w", translateNotFound(err))
	}
	return row, nil
}

func (s *Store) ListIdentityLinksForUser(ctx context.Context, userID string) ([]IdentityLinkWithProviderRow, error) {
	rows, err := s.queries.ListIdentityLinksForUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("identity_link: list: %w", err)
	}
	return rows, nil
}

func (s *Store) IsTokenRevoked(ctx context.Context, jti string) (bool, error) {
	revoked, err := s.queries.IsTokenRevoked(ctx, jti)
	if err != nil {
		return false, fmt.Errorf("revoked_token: lookup: %w", err)
	}
	return revoked, nil
}

func (s *Store) GetServerSettings(ctx context.Context) (ServerSettingsRow, error) {
	row, err := s.queries.GetServerSettings(ctx)
	if err != nil {
		return ServerSettingsRow{}, fmt.Errorf("server_settings: get: %w", translateNotFound(err))
	}
	return row, nil
}

func (s *Store) CountLiveBootstrapAdminTokens(ctx context.Context, at time.Time) (int64, error) {
	at = at.UTC()
	n, err := s.queries.CountLiveBootstrapAdminTokens(ctx, generated.CountLiveBootstrapAdminTokensParams{
		ReservedName: BootstrapAdminTokenName,
		Now:          at,
	})
	if err != nil {
		return 0, fmt.Errorf("bootstrap_token: count: %w", err)
	}
	return n, nil
}

const BootstrapAdminTokenName = "bootstrap-admin"

type AuthStateRow = generated.AuthState
