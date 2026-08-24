package store_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/manchtools/cadestro/server/internal/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/server/internal/execution"
	"github.com/manchtools/cadestro/server/internal/store"
)

type executionResultFixture struct {
	t          *testing.T
	store      *store.Store
	raw        *testdb.DB
	service    *execution.Service
	now        time.Time
	deviceID   string
	deliveryID string
	manifestID string
	execution  string
	actionID   string
}

func newExecutionResultFixture(t *testing.T, deliveryState, executionState string) *executionResultFixture {
	t.Helper()
	st, raw := setupSQLite(t)
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	f := &executionResultFixture{
		t: t, store: st, raw: raw, now: now,
		deviceID: newID(), deliveryID: newID(), manifestID: newID(),
		execution: newID(), actionID: newID(),
	}
	_, err := raw.Exec(context.Background(), `
		INSERT INTO devices (id, hostname, agent_version, registered_at)
		VALUES ($1, 'device', 'v1', $2)`, f.deviceID, now)
	require.NoError(t, err)
	manifest, err := protojson.Marshal(&cadestrov1.Manifest{
		ManifestId: f.manifestID,
		Occurrences: []*cadestrov1.ManifestOccurrence{{
			OccurrenceId: f.execution,
			Action:       &cadestrov1.Action{Id: &cadestrov1.ActionId{Value: f.actionID}, Type: cadestrov1.ActionType_ACTION_TYPE_UPDATE},
		}},
	})
	require.NoError(t, err)
	_, err = raw.Exec(context.Background(), `
		INSERT INTO deliveries (
			delivery_id, device_id, manifest_id, manifest, state
		) VALUES ($1, $2, $3, $4, $5)`,
		f.deliveryID, f.deviceID, f.manifestID, manifest, deliveryState)
	require.NoError(t, err)
	_, err = raw.Exec(context.Background(), `
		INSERT INTO executions (
			id, delivery_id, device_id, action_type, desired_state, params,
			timeout_seconds, status, created_at, created_by_type, created_by_id
		) VALUES ($1, $2, $3, 1, 0, '{}', 300, $4, $5, 'user', $6)`,
		f.execution, f.deliveryID, f.deviceID, executionState, now, newID())
	require.NoError(t, err)
	f.service = execution.New(execution.Config{Store: st, Now: func() time.Time { return now }})
	return f
}

func (f *executionResultFixture) result(status cadestrov1.ExecutionStatus) *cadestrov1.ActionResult {
	f.t.Helper()
	return &cadestrov1.ActionResult{
		ActionId: &cadestrov1.ActionId{Value: f.actionID}, Status: status,
		DeliveryId: f.deliveryID, OccurrenceId: f.execution,
	}
}

func TestExecutionResult_CommitsTerminalStateAndAbsorbsReplay(t *testing.T) {
	f := newExecutionResultFixture(t, "PENDING", "pending")
	completed := f.now.Add(-time.Minute)
	result := f.result(cadestrov1.ExecutionStatus_EXECUTION_STATUS_SUCCESS)
	result.CompletedAt = timestamppb.New(completed)
	result.DurationMs = 1234
	result.Changed = true
	result.Compliant = true
	result.Output = &cadestrov1.CommandOutput{ExitCode: 0, Stdout: "done"}
	result.DetectionOutput = &cadestrov1.CommandOutput{ExitCode: 0, Stdout: "compliant"}

	require.NoError(t, f.service.ApplyActionResult(context.Background(), f.deviceID, result))
	row, err := f.store.GetExecution(context.Background(), f.execution)
	require.NoError(t, err)
	assert.Equal(t, "success", row.Status)
	assert.True(t, row.Changed)
	assert.True(t, row.Compliant)
	require.NotNil(t, row.CompletedAt)
	assert.True(t, row.CompletedAt.Equal(completed))
	require.NotNil(t, row.DurationMs)
	assert.Equal(t, int64(1234), *row.DurationMs)
	assert.JSONEq(t, `{"stdout":"done"}`, string(row.Output))
	assert.JSONEq(t, `{"stdout":"compliant"}`, string(row.DetectionOutput))

	var before int
	require.NoError(t, f.raw.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM audit_operations WHERE request_descriptor = 'execution.result'`).Scan(&before))
	require.NoError(t, f.service.ApplyActionResult(context.Background(), f.deviceID, result))
	var after int
	require.NoError(t, f.raw.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM audit_operations WHERE request_descriptor = 'execution.result'`).Scan(&after))
	assert.Equal(t, before, after, "an identical replay is not another mutation")

	conflict := f.result(cadestrov1.ExecutionStatus_EXECUTION_STATUS_FAILED)
	conflict.Error = "different outcome"
	err = f.service.ApplyActionResult(context.Background(), f.deviceID, conflict)
	assert.ErrorIs(t, err, execution.ErrConflictingReplay)
}

func TestExecutionResult_RunningThenIndeterminate(t *testing.T) {
	f := newExecutionResultFixture(t, "PENDING", "pending")
	require.NoError(t, f.service.ApplyActionResult(context.Background(), f.deviceID,
		f.result(cadestrov1.ExecutionStatus_EXECUTION_STATUS_RUNNING)))
	row, err := f.store.GetExecution(context.Background(), f.execution)
	require.NoError(t, err)
	assert.Equal(t, "running", row.Status)

	indeterminate := f.result(cadestrov1.ExecutionStatus_EXECUTION_STATUS_INDETERMINATE)
	indeterminate.Error = "agent restarted after STARTED"
	require.NoError(t, f.service.ApplyActionResult(context.Background(), f.deviceID, indeterminate))
	row, err = f.store.GetExecution(context.Background(), f.execution)
	require.NoError(t, err)
	assert.Equal(t, "indeterminate", row.Status)
}

func TestExecutionResult_EnforcesIdentityBindings(t *testing.T) {
	f := newExecutionResultFixture(t, "PENDING", "pending")
	result := f.result(cadestrov1.ExecutionStatus_EXECUTION_STATUS_SUCCESS)
	assert.NoError(t, f.service.ApplyActionResult(context.Background(), f.deviceID, result))

	wrongDevice := newID()
	assert.ErrorIs(t, f.service.ApplyActionResult(context.Background(), wrongDevice, result), execution.ErrWrongDevice)

	f2 := newExecutionResultFixture(t, "PENDING", "pending")
	wrongAction := f2.result(cadestrov1.ExecutionStatus_EXECUTION_STATUS_SUCCESS)
	wrongAction.ActionId.Value = newID()
	assert.ErrorIs(t, f2.service.ApplyActionResult(context.Background(), f2.deviceID, wrongAction), execution.ErrWrongAction)

	wrongDelivery := f2.result(cadestrov1.ExecutionStatus_EXECUTION_STATUS_SUCCESS)
	wrongDelivery.DeliveryId = newID()
	assert.ErrorIs(t, f2.service.ApplyActionResult(context.Background(), f2.deviceID, wrongDelivery), execution.ErrWrongDelivery)

	invalid := f2.result(cadestrov1.ExecutionStatus_EXECUTION_STATUS_SUCCESS)
	invalid.Metadata = map[string]string{"lps.rotations": "must never ride result metadata"}
	assert.ErrorIs(t, f2.service.ApplyActionResult(context.Background(), f2.deviceID, invalid), execution.ErrInvalidInput)
}

func TestExecutionOutputChunk_IsBoundedOwnedAndIdempotent(t *testing.T) {
	f := newExecutionResultFixture(t, "PENDING", "running")
	chunk := &cadestrov1.OutputChunk{
		ExecutionId: f.execution, Stream: cadestrov1.OutputStreamType_OUTPUT_STREAM_TYPE_STDOUT,
		Data: []byte("hello"), Sequence: 4,
	}
	require.NoError(t, f.service.AppendOutputChunk(context.Background(), f.deviceID, chunk))
	require.NoError(t, f.service.AppendOutputChunk(context.Background(), f.deviceID, chunk))
	conflict := proto.Clone(chunk).(*cadestrov1.OutputChunk)
	conflict.Data = []byte("different")
	assert.ErrorIs(t, f.service.AppendOutputChunk(context.Background(), f.deviceID, conflict), execution.ErrConflictingReplay)
	var count int
	require.NoError(t, f.raw.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM execution_output_chunks WHERE execution_id = $1`, f.execution).Scan(&count))
	assert.Equal(t, 1, count)

	err := f.service.AppendOutputChunk(context.Background(), newID(), chunk)
	assert.ErrorIs(t, err, execution.ErrWrongDevice)
	oversized := proto.Clone(chunk).(*cadestrov1.OutputChunk)
	oversized.Data = bytes.Repeat([]byte{'x'}, 64*1024+1)
	assert.ErrorIs(t, f.service.AppendOutputChunk(context.Background(), f.deviceID, oversized), execution.ErrInvalidInput)
}

// TestExecutionOutputChunk_PerExecutionBudget pins the inbox trust-boundary
// budget: the per-chunk 64 KiB bound alone lets a device grow the database
// without limit by streaming ever-higher sequence numbers. The sequence cap
// bounds one execution's stored output at MaxOutputChunks × 64 KiB, rejected
// in validation before any read or write; the stream loop's log-and-continue
// error handling keeps the agent connected through the rejection.
func TestExecutionOutputChunk_PerExecutionBudget(t *testing.T) {
	f := newExecutionResultFixture(t, "PENDING", "running")
	inBudget := &cadestrov1.OutputChunk{
		ExecutionId: f.execution, Stream: cadestrov1.OutputStreamType_OUTPUT_STREAM_TYPE_STDOUT,
		Data: []byte("tail"), Sequence: execution.MaxOutputChunks - 1,
	}
	require.NoError(t, f.service.AppendOutputChunk(context.Background(), f.deviceID, inBudget),
		"the last in-budget sequence must stay accepted")

	over := proto.Clone(inBudget).(*cadestrov1.OutputChunk)
	over.Data = []byte("over")
	over.Sequence = execution.MaxOutputChunks
	assert.ErrorIs(t, f.service.AppendOutputChunk(context.Background(), f.deviceID, over),
		execution.ErrInvalidInput, "the first over-budget sequence must be rejected")

	far := proto.Clone(inBudget).(*cadestrov1.OutputChunk)
	far.Data = []byte("far")
	far.Sequence = execution.MaxOutputChunks * 100
	assert.ErrorIs(t, f.service.AppendOutputChunk(context.Background(), f.deviceID, far),
		execution.ErrInvalidInput, "a runaway sequence must be rejected")

	var count int
	require.NoError(t, f.raw.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM execution_output_chunks WHERE execution_id = $1`, f.execution).Scan(&count))
	assert.Equal(t, 1, count, "over-budget chunks must write nothing")
}

func TestExecutionResult_RejectsMalformedAndCancelledTransitions(t *testing.T) {
	f := newExecutionResultFixture(t, "PENDING", "cancelled")
	err := f.service.ApplyActionResult(context.Background(), f.deviceID,
		f.result(cadestrov1.ExecutionStatus_EXECUTION_STATUS_SUCCESS))
	assert.True(t, errors.Is(err, execution.ErrInvalidTransition) || errors.Is(err, execution.ErrConflictingReplay))

	malformed := f.result(cadestrov1.ExecutionStatus_EXECUTION_STATUS_RUNNING)
	malformed.Error = "running is not terminal"
	assert.ErrorIs(t, f.service.ApplyActionResult(context.Background(), f.deviceID, malformed), execution.ErrInvalidInput)
}
