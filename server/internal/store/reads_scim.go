package store

import (
	"context"
	"fmt"

	"github.com/manchtools/cadestro/server/internal/store/generated"
)

type (
	UserGroupRow = generated.UserGroup

	SCIMGroupMappingRow = generated.ScimGroupMapping
)

type SCIMUserRow struct {
	User       UserRow
	ExternalID string
}

func (s *Store) ListSCIMUsers(ctx context.Context, providerID string, limit, offset int32) ([]SCIMUserRow, error) {
	rows, err := s.queries.ListSCIMUsers(ctx, generated.ListSCIMUsersParams{
		ProviderID: providerID,
		Limit:      int64(limit),
		Offset:     int64(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("scim: list users: %w", err)
	}
	out := make([]SCIMUserRow, len(rows))
	for i, r := range rows {
		out[i] = SCIMUserRow{User: r.User, ExternalID: r.ExternalID}
	}
	return out, nil
}

func (s *Store) CountSCIMUsers(ctx context.Context, providerID string) (int64, error) {
	n, err := s.queries.CountSCIMUsers(ctx, providerID)
	if err != nil {
		return 0, fmt.Errorf("scim: count users: %w", err)
	}
	return n, nil
}

func (s *Store) FindSCIMUserByEmail(ctx context.Context, providerID, email string) (SCIMUserRow, error) {
	row, err := s.queries.FindSCIMUserByEmail(ctx, generated.FindSCIMUserByEmailParams{
		ProviderID: providerID,
		Email:      email,
	})
	if err != nil {
		return SCIMUserRow{}, fmt.Errorf("scim: find user by email: %w", translateNotFound(err))
	}
	return SCIMUserRow{User: row.User, ExternalID: row.ExternalID}, nil
}

func (s *Store) FindSCIMUserByExternalID(ctx context.Context, providerID, externalID string) (SCIMUserRow, error) {
	row, err := s.queries.FindSCIMUserByExternalID(ctx, generated.FindSCIMUserByExternalIDParams{
		ProviderID: providerID,
		ExternalID: externalID,
	})
	if err != nil {
		return SCIMUserRow{}, fmt.Errorf("scim: find user by external id: %w", translateNotFound(err))
	}
	return SCIMUserRow{User: row.User, ExternalID: row.ExternalID}, nil
}

func (s *Store) GetIdentityLinkByProviderAndUser(ctx context.Context, providerID, userID string) (IdentityLinkRow, error) {
	row, err := s.queries.GetIdentityLinkByProviderAndUser(ctx, generated.GetIdentityLinkByProviderAndUserParams{
		ProviderID: providerID,
		UserID:     userID,
	})
	if err != nil {
		return IdentityLinkRow{}, fmt.Errorf("identity_link: get by provider and user: %w", translateNotFound(err))
	}
	return row, nil
}

func (s *Store) CountIdentityLinksForUser(ctx context.Context, userID string) (int64, error) {
	n, err := s.queries.CountIdentityLinksForUser(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("identity_link: count for user: %w", err)
	}
	return n, nil
}

func (s *Store) GetUserGroup(ctx context.Context, id string) (UserGroupRow, error) {
	row, err := s.queries.GetUserGroup(ctx, id)
	if err != nil {
		return UserGroupRow{}, fmt.Errorf("user_group: get: %w", translateNotFound(err))
	}
	return row, nil
}

func (s *Store) ListUserGroupMemberIDs(ctx context.Context, groupID string) ([]string, error) {
	rows, err := s.queries.ListUserGroupMemberIDs(ctx, groupID)
	if err != nil {
		return nil, fmt.Errorf("user_group: list members: %w", err)
	}
	return rows, nil
}

func (s *Store) GetSCIMGroupMapping(ctx context.Context, providerID, scimGroupID string) (SCIMGroupMappingRow, error) {
	row, err := s.queries.GetSCIMGroupMapping(ctx, generated.GetSCIMGroupMappingParams{
		ProviderID:  providerID,
		ScimGroupID: scimGroupID,
	})
	if err != nil {
		return SCIMGroupMappingRow{}, fmt.Errorf("scim_group_mapping: get: %w", translateNotFound(err))
	}
	return row, nil
}

func (s *Store) GetSCIMGroupMappingByUserGroup(ctx context.Context, providerID, userGroupID string) (SCIMGroupMappingRow, error) {
	row, err := s.queries.GetSCIMGroupMappingByUserGroup(ctx, generated.GetSCIMGroupMappingByUserGroupParams{
		ProviderID:  providerID,
		UserGroupID: userGroupID,
	})
	if err != nil {
		return SCIMGroupMappingRow{}, fmt.Errorf("scim_group_mapping: get by user group: %w", translateNotFound(err))
	}
	return row, nil
}

func (s *Store) ListSCIMGroupMappings(ctx context.Context, providerID string) ([]SCIMGroupMappingRow, error) {
	rows, err := s.queries.ListSCIMGroupMappings(ctx, providerID)
	if err != nil {
		return nil, fmt.Errorf("scim_group_mapping: list: %w", err)
	}
	return rows, nil
}
