package store

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"time"

	db "github.com/manchtools/cadestro/server/internal/store/generated"
)

const heartbeatBatchSize = 256

func (s *Store) RecordHeartbeatTelemetry(ctx context.Context, snapshot map[string]time.Time) error {
	if ctx == nil || s == nil {
		return errors.New("heartbeat telemetry requires a store and context")
	}
	ids := make([]string, 0, len(snapshot))
	for id, at := range snapshot {
		if id == "" || at.IsZero() {
			return errors.New("heartbeat telemetry contains an invalid device")
		}
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for batch := range slices.Chunk(ids, heartbeatBatchSize) {
		if err := s.withTx(ctx, func(_ *sql.Tx, queries *db.Queries) error {
			for _, id := range batch {
				seenAt := snapshot[id].UTC().Truncate(time.Microsecond)
				if _, err := queries.RecordDeviceHeartbeat(ctx, db.RecordDeviceHeartbeatParams{
					DeviceID: id, LastSeenAt: &seenAt,
				}); err != nil {
					return err
				}
				if err := refreshSearchDocument(ctx, queries, "devices", id); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}
