package store

import (
	"context"
	"fmt"

	db "github.com/manchtools/cadestro/server/internal/store/generated"
)

type RegistrationTokenRow struct {
	db.Token
	CurrentUses int32
}

type RegistrationTokenListFilter struct {
	AfterID         string
	Limit           int32
	IncludeDisabled bool
}

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

func (s *Store) CountRegistrationTokens(ctx context.Context, includeDisabled bool) (int64, error) {
	n, err := s.queries.CountRegistrationTokens(ctx, db.CountRegistrationTokensParams{
		ReservedName: BootstrapAdminTokenName, IncludeDisabled: includeDisabled,
	})
	if err != nil {
		return 0, fmt.Errorf("registration token: count: %w", err)
	}
	return n, nil
}
