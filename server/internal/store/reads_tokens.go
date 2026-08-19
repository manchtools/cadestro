package store

import (
	"context"
	"fmt"

	db "github.com/manchtools/cadestro/server/internal/store/generated"
)

// RegistrationTokenRow is the non-secret token shape. CurrentUses is derived
// from device provenance and is never written back to tokens.
// value_hash remains internal to the state service and is never copied onto
// the protobuf read surface.
type RegistrationTokenRow struct {
	db.Token
	CurrentUses int32
}

// RegistrationTokenListFilter is the deterministic keyset page requested by
// the registration-token RPC surface.
type RegistrationTokenListFilter struct {
	AfterID         string
	Limit           int32
	IncludeDisabled bool
}

// ListRegistrationTokens returns live non-bootstrap tokens in id order.
func (s *Store) ListRegistrationTokens(ctx context.Context, f RegistrationTokenListFilter) ([]RegistrationTokenRow, error) {
	rows, err := s.queries.ListRegistrationTokens(ctx, db.ListRegistrationTokensParams{
		ReservedName: BootstrapAdminTokenName, AfterID: f.AfterID,
		IncludeDisabled: f.IncludeDisabled, RowLimit: int64(f.Limit),
	})
	if err != nil {
		return nil, fmt.Errorf("registration token: list: %w", err)
	}
	out := make([]RegistrationTokenRow, len(rows))
	for i, row := range rows {
		out[i] = RegistrationTokenRow{Token: db.Token{
			ID: row.ID, ValueHash: row.ValueHash, Name: row.Name, MaxUses: row.MaxUses,
			ExpiresAt: row.ExpiresAt, CreatedAt: row.CreatedAt, CreatedBy: row.CreatedBy,
			Disabled: row.Disabled, IsDeleted: row.IsDeleted,
		}, CurrentUses: int32(row.CurrentUses)}
	}
	return out, nil
}

// CountRegistrationTokens counts the same live token population as the list.
func (s *Store) CountRegistrationTokens(ctx context.Context, includeDisabled bool) (int64, error) {
	n, err := s.queries.CountRegistrationTokens(ctx, db.CountRegistrationTokensParams{
		ReservedName: BootstrapAdminTokenName, IncludeDisabled: includeDisabled,
	})
	if err != nil {
		return 0, fmt.Errorf("registration token: count: %w", err)
	}
	return n, nil
}
