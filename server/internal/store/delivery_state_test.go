package store_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/manchtools/cadestro/server/internal/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	pmcrypto "github.com/manchtools/cadestro/server/internal/crypto"
	"github.com/manchtools/cadestro/server/internal/delivery"
	"github.com/manchtools/cadestro/server/internal/store"
)

type deliveryFixture struct {
	t          *testing.T
	store      *store.Store
	raw        *testdb.DB
	now        time.Time
	deviceID   string
	manifest   *cadestrov1.Manifest
	deliveryID string
	service    *delivery.Service
	atRest     *pmcrypto.Encryptor
}

func newDeliveryFixture(t *testing.T) *deliveryFixture {
	t.Helper()
	st, raw := setupSQLite(t)
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	atRest, err := pmcrypto.NewEncryptor(strings.Repeat("01", 32))
	require.NoError(t, err)
	deviceID := seedDevice(t, raw)
	actionID, manifestID := newID(), newID()
	manifest := &cadestrov1.Manifest{
		ManifestId: manifestID,
		Provenance: &cadestrov1.ManifestProvenance{ActionId: actionID},
		Schedule:   &cadestrov1.ActionSchedule{RunOnAssign: true},
		Occurrences: []*cadestrov1.ManifestOccurrence{{
			OccurrenceId: newID(),
			Action: &cadestrov1.Action{
				Id: &cadestrov1.ActionId{Value: actionID}, Type: cadestrov1.ActionType_ACTION_TYPE_UPDATE,
			},
		}},
	}
	op := mutationOp()
	op.OperationID = newID()
	op.RequestDescriptor = "cadestro.v1.ControlService/DispatchAction"
	var deliveryID string
	_, err = st.WithAudit(context.Background(), op, func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
		var err error
		deliveryID, err = delivery.InsertInTx(ctx, tx, rec, delivery.InsertParams{
			OperationID: op.OperationID, DeviceID: deviceID, Manifest: manifest, AvailableAt: now,
		})
		return err
	})
	require.NoError(t, err)
	return &deliveryFixture{
		t: t, store: st, raw: raw, now: now, deviceID: deviceID, manifest: manifest, deliveryID: deliveryID,
		service: delivery.New(delivery.Config{Store: st, Now: func() time.Time { return now }}), atRest: atRest,
	}
}

func TestDelivery_InsertCommitsCompleteManifestWithAudit(t *testing.T) {
	f := newDeliveryFixture(t)
	row, err := f.store.GetDelivery(context.Background(), f.deliveryID)
	require.NoError(t, err)
	assert.Equal(t, "PENDING", row.State)
	assert.Equal(t, f.deviceID, row.DeviceID)
	assert.Equal(t, f.manifest.ManifestId, row.ManifestID)
	require.NotNil(t, row.OperationID)

	var stored cadestrov1.Manifest
	require.NoError(t, protojson.Unmarshal(row.Manifest, &stored))
	assert.True(t, proto.Equal(f.manifest, &stored), "the durable row must carry the complete manifest")
	effects, err := f.store.ListAuditEffects(context.Background(), *row.OperationID)
	require.NoError(t, err)
	require.Len(t, effects, 1)
	assert.Equal(t, f.deliveryID, effects[0].ResourceID)
	assert.Equal(t, "CREATE", effects[0].Action)
}

func TestDelivery_InsertRejectsAmbiguousOrDuplicateManifestIdentity(t *testing.T) {
	f := newDeliveryFixture(t)
	tests := map[string]func(*cadestrov1.Manifest){
		"ambiguous provenance": func(manifest *cadestrov1.Manifest) {
			manifest.Provenance.ActionSetId = newID()
		},
		"duplicate occurrence": func(manifest *cadestrov1.Manifest) {
			manifest.Occurrences = append(manifest.Occurrences, proto.Clone(manifest.Occurrences[0]).(*cadestrov1.ManifestOccurrence))
		},
		"missing nested action id": func(manifest *cadestrov1.Manifest) {
			manifest.Occurrences[0].Action.Id.Value = ""
		},
	}
	for name, breakManifest := range tests {
		t.Run(name, func(t *testing.T) {
			manifest := proto.Clone(f.manifest).(*cadestrov1.Manifest)
			manifest.ManifestId = newID()
			breakManifest(manifest)
			op := mutationOp()
			op.OperationID = newID()
			_, err := f.store.WithAudit(context.Background(), op, func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
				_, err := delivery.InsertInTx(ctx, tx, rec, delivery.InsertParams{
					OperationID: op.OperationID, DeviceID: f.deviceID, Manifest: manifest, AvailableAt: f.now,
				})
				return err
			})
			assert.ErrorIs(t, err, delivery.ErrInvalidInput)
		})
	}
	var count int
	require.NoError(t, f.raw.QueryRow(context.Background(), `SELECT count(*) FROM deliveries`).Scan(&count))
	assert.Equal(t, 1, count, "rejected manifests must not create delivery rows")
}

func TestDelivery_ResultReplayIsIdempotentWithoutTransportReceipt(t *testing.T) {
	f := newDeliveryFixture(t)
	ctx := context.Background()

	changed, err := f.service.Complete(ctx, f.deliveryID, f.deviceID, f.manifest.ManifestId, delivery.StateSucceeded, "OK")
	require.NoError(t, err)
	assert.True(t, changed)
	changed, err = f.service.Complete(ctx, f.deliveryID, f.deviceID, f.manifest.ManifestId, delivery.StateSucceeded, "OK")
	require.NoError(t, err)
	assert.False(t, changed, "result replay must be absorbed")
}
