package store_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	pmv1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/server/internal/store"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestPolicyResultsAreOwnerBoundAndReplaySafe(t *testing.T) {
	st, raw := setupSQLite(t)
	deviceID, runID, occurrenceID, actionID := ulid.Make().String(), ulid.Make().String(), ulid.Make().String(), ulid.Make().String()
	_, err := raw.Exec(context.Background(), `INSERT INTO devices (id, hostname, agent_version, agent_sealing_public_key, registered_at) VALUES ($1, 'device', 'v1', $2, $3)`, deviceID, bytes.Repeat([]byte{1}, 32), time.Now())
	require.NoError(t, err)
	result := &pmv1.ActionResult{
		ActionId: &pmv1.ActionId{Value: actionID}, DeliveryId: runID, OccurrenceId: occurrenceID,
		Status: pmv1.ExecutionStatus_EXECUTION_STATUS_SUCCESS,
	}
	require.NoError(t, st.RecordPolicyActionResult(context.Background(), deviceID, result))
	require.NoError(t, st.RecordPolicyActionResult(context.Background(), deviceID, result))
	changed := proto.Clone(result).(*pmv1.ActionResult)
	changed.Status = pmv1.ExecutionStatus_EXECUTION_STATUS_FAILED
	require.ErrorIs(t, st.RecordPolicyActionResult(context.Background(), deviceID, changed), store.ErrPolicyResultConflict)
	require.ErrorIs(t, st.RecordPolicyActionResult(context.Background(), ulid.Make().String(), result), store.ErrPolicyResultConflict)

	manifestID := ulid.Make().String()
	require.NoError(t, st.RecordPolicyManifestResult(context.Background(), deviceID, runID, manifestID, "SUCCEEDED", "OK"))
	require.NoError(t, st.RecordPolicyManifestResult(context.Background(), deviceID, runID, manifestID, "SUCCEEDED", "OK"))
	require.ErrorIs(t, st.RecordPolicyManifestResult(context.Background(), deviceID, runID, ulid.Make().String(), "SUCCEEDED", "OK"), store.ErrPolicyResultConflict)
}
