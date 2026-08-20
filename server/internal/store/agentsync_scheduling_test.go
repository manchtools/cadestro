package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/manchtools/cadestro/server/internal/agentsync"
	"github.com/manchtools/cadestro/server/internal/connection"
)

// connectedSyncer registers a live connection for the fixture device and
// returns a syncer whose clock is pinned to the fixture timeline so
// future-scheduled availability is meaningful.
func connectedSyncer(t *testing.T, f *deliveryFixture) (*agentsync.Service, *connection.Agent) {
	t.Helper()
	manager := connection.NewManager()
	agent := manager.Register(context.Background(), f.deviceID, "device", "v1", nil)
	t.Cleanup(agent.Close)
	syncer := agentsync.New(agentsync.Config{
		Store: f.store, Manager: manager,
		Now:    func() time.Time { return f.now },
		AtRest: f.atRest,
	})
	return syncer, agent
}

func offeredDeliveryIDs(t *testing.T, syncer *agentsync.Service, deviceID string) map[string]struct{} {
	t.Helper()
	resp, err := syncer.Sync(context.Background(), deviceID)
	require.NoError(t, err)
	ids := make(map[string]struct{}, len(resp.Deliveries))
	for _, d := range resp.Deliveries {
		ids[d.DeliveryId] = struct{}{}
	}
	return ids
}

// TestAgentSync_DoesNotPullFutureScheduledDeliveriesForward proves the sync
// path honours delivery availability: a future row is not offered, while a
// due row is pulled and remains safely repeatable until its terminal result.
func TestAgentSync_DoesNotPullFutureScheduledDeliveriesForward(t *testing.T) {
	ctx := context.Background()

	t.Run("future pending stays scheduled and untouched", func(t *testing.T) {
		f := newDeliveryFixture(t)
		syncer, _ := connectedSyncer(t, f)
		payload, err := protojson.Marshal(f.manifest)
		require.NoError(t, err)

		futureID := newID()
		_, err = f.raw.Exec(ctx, `
			INSERT INTO deliveries (delivery_id, device_id, manifest_id, manifest, state, available_at)
			VALUES ($1, $2, $3, $4, 'PENDING', $5)`,
			futureID, f.deviceID, f.manifest.ManifestId, payload, f.now.Add(24*time.Hour))
		require.NoError(t, err)

		offered := offeredDeliveryIDs(t, syncer, f.deviceID)
		_, present := offered[futureID]
		assert.False(t, present, "a future-scheduled delivery must not be pulled forward")

		row, err := f.store.GetDelivery(ctx, futureID)
		require.NoError(t, err)
		assert.Equal(t, "PENDING", row.State, "a future delivery must remain PENDING")
	})

	t.Run("due pending is offered and repeatable", func(t *testing.T) {
		f := newDeliveryFixture(t)
		syncer, _ := connectedSyncer(t, f)

		offered := offeredDeliveryIDs(t, syncer, f.deviceID)
		_, present := offered[f.deliveryID]
		assert.True(t, present, "a due delivery must be offered")

		row, err := f.store.GetDelivery(ctx, f.deliveryID)
		require.NoError(t, err)
		assert.Equal(t, "PENDING", row.State)
		repeated := offeredDeliveryIDs(t, syncer, f.deviceID)
		_, present = repeated[f.deliveryID]
		assert.True(t, present, "repeated Sync must remain safe before the terminal result")
	})
}

func TestListDueDeviceDeliveriesDoesNotLetFutureWorkHideDueWork(t *testing.T) {
	f := newDeliveryFixture(t)
	ctx := context.Background()
	_, err := f.raw.Exec(ctx, `UPDATE deliveries SET available_at = $1 WHERE delivery_id = $2`,
		f.now.Add(24*time.Hour), f.deliveryID)
	require.NoError(t, err)

	payload, err := protojson.Marshal(f.manifest)
	require.NoError(t, err)
	dueID := newID()
	_, err = f.raw.Exec(ctx, `
		INSERT INTO deliveries (delivery_id, device_id, manifest_id, manifest, state, created_at, available_at)
		VALUES ($1, $2, $3, $4, 'PENDING', $5, $6)`,
		dueID, f.deviceID, f.manifest.ManifestId, payload, f.now.Add(time.Minute), f.now)
	require.NoError(t, err)

	rows, err := f.store.ListDueDeviceDeliveries(ctx, f.deviceID, f.now, 1)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, dueID, rows[0].DeliveryID)
}
