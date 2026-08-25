package store

import (
	"context"
	"fmt"

	"github.com/manchtools/cadestro/server/internal/store/generated"
)

func RefreshDeviceCompliance(ctx context.Context, tx *Tx, rec *AuditRecorder, deviceIDs ...[]string) error {
	seen := make(map[string]struct{})
	for _, ids := range deviceIDs {
		for _, id := range ids {
			if _, done := seen[id]; done {
				continue
			}
			seen[id] = struct{}{}
			if _, err := tx.RefreshDeviceComplianceStatus(ctx, generated.RefreshDeviceComplianceStatusParams{
				DeviceID: id,
			}); err != nil {
				return fmt.Errorf("refresh device compliance: %w", err)
			}
			rec.RefreshSearch("device", id)
		}
	}
	return nil
}
