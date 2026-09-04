package store

import (
	"context"
	"testing"
	"time"

	contract "github.com/manchtools/cadestro/contract"
	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func testAction(id, script string) *pb.Action {
	return &pb.Action{Id: &pb.ActionId{Value: id}, Params: &pb.Action_Shell{Shell: &pb.ShellActionParams{Script: script}}, Schedule: &pb.ActionSchedule{IntervalHours: 1}}
}

func testPolicy(actions ...*pb.Action) *pb.DesiredPolicy {
	return &pb.DesiredPolicy{Actions: actions}
}

func testResult(t *testing.T, action *pb.Action, runID string, completed time.Time) *pb.ActionResult {
	t.Helper()
	digest, err := contract.ActionDigest(action)
	require.NoError(t, err)
	return &pb.ActionResult{ActionId: action.Id, RunId: &pb.RunId{Value: runID}, Status: pb.ExecutionStatus_EXECUTION_STATUS_SUCCESS, CompletedAt: timestamppb.New(completed), ActionDigest: digest}
}

func TestReconcileActionUpdatesOnlyChangedRows(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	st, err := New(t.TempDir())
	require.NoError(t, err)
	defer st.Close()
	st.now = func() time.Time { return now }
	id := "01K00000000000000000000001"
	action := testAction(id, "true")
	require.NoError(t, st.ReconcilePolicy(ctx, testPolicy(action)))
	due, err := st.GetDueScheduledWork(ctx)
	require.NoError(t, err)
	require.Len(t, due, 1)
	require.NoError(t, st.BeginActionRun(ctx, &due[0], now))
	_, err = st.RecordActionResult(ctx, testResult(t, due[0].Action, due[0].RunID, now))
	require.NoError(t, err)
	changed := testAction(id, "false")
	now = now.Add(10 * time.Minute)
	require.NoError(t, st.ReconcilePolicy(ctx, testPolicy(changed)))
	due, err = st.GetDueScheduledWork(ctx)
	require.NoError(t, err)
	require.Len(t, due, 1)
	require.Equal(t, "false", due[0].Action.GetShell().GetScript())
	next, err := st.queries.GetScheduledWorkNextExecute(ctx, id)
	require.NoError(t, err)
	now = now.Add(time.Minute)
	require.NoError(t, st.ReconcilePolicy(ctx, testPolicy(changed)))
	due, err = st.GetDueScheduledWork(ctx)
	require.NoError(t, err)
	require.Len(t, due, 1)
	nextAfter, err := st.queries.GetScheduledWorkNextExecute(ctx, id)
	require.NoError(t, err)
	require.Equal(t, next, nextAfter)
}

func TestBeginActionRunLoadsCurrentAction(t *testing.T) {
	ctx := context.Background()
	st, err := New(t.TempDir())
	require.NoError(t, err)
	defer st.Close()
	id := "01K00000000000000000000007"
	require.NoError(t, st.ReconcilePolicy(ctx, testPolicy(testAction(id, "true"))))
	due, err := st.GetDueScheduledWork(ctx)
	require.NoError(t, err)
	require.Len(t, due, 1)
	require.NoError(t, st.ReconcilePolicy(ctx, testPolicy(testAction(id, "false"))))
	require.NoError(t, st.BeginActionRun(ctx, &due[0], time.Now()))
	require.Equal(t, "false", due[0].Action.GetShell().GetScript())
}

func TestRecordActionResultBindsExactRunAndDigest(t *testing.T) {
	ctx := context.Background()
	st, err := New(t.TempDir())
	require.NoError(t, err)
	defer st.Close()
	action := testAction("01K00000000000000000000011", "true")
	require.NoError(t, st.ReconcilePolicy(ctx, testPolicy(action)))
	due, err := st.GetDueScheduledWork(ctx)
	require.NoError(t, err)
	require.NoError(t, st.BeginActionRun(ctx, &due[0], time.Now()))
	wrong := testResult(t, testAction(action.GetId().GetValue(), "false"), due[0].RunID, time.Now())
	_, err = st.RecordActionResult(ctx, wrong)
	require.Error(t, err)
	_, err = st.RecordActionResult(ctx, testResult(t, due[0].Action, due[0].RunID, time.Now()))
	require.NoError(t, err)
}

func TestActionResultOutboxAckDeletesOne(t *testing.T) {
	ctx := context.Background()
	st, err := New(t.TempDir())
	require.NoError(t, err)
	defer st.Close()
	action := testAction("01K00000000000000000000021", "true")
	require.NoError(t, st.ReconcilePolicy(ctx, testPolicy(action)))
	due, err := st.GetDueScheduledWork(ctx)
	require.NoError(t, err)
	require.NoError(t, st.BeginActionRun(ctx, &due[0], time.Now()))
	sequence, err := st.RecordActionResult(ctx, testResult(t, due[0].Action, due[0].RunID, time.Now()))
	require.NoError(t, err)
	pending, err := st.GetPendingResults(ctx)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	require.Equal(t, sequence, pending[0].Sequence)
	require.NoError(t, st.DeletePendingResult(ctx, sequence))
	pending, err = st.GetPendingResults(ctx)
	require.NoError(t, err)
	require.Empty(t, pending)
}

func TestInterruptedRunKeepsExecutedActionDigest(t *testing.T) {
	ctx := context.Background()
	st, err := New(t.TempDir())
	require.NoError(t, err)
	defer st.Close()
	id := "01K00000000000000000000031"
	executed := testAction(id, "true")
	require.NoError(t, st.ReconcilePolicy(ctx, testPolicy(executed)))
	due, err := st.GetDueScheduledWork(ctx)
	require.NoError(t, err)
	require.NoError(t, st.BeginActionRun(ctx, &due[0], time.Now()))
	require.NoError(t, st.ReconcilePolicy(ctx, testPolicy(testAction(id, "false"))))
	recovered, err := st.RecoverInterruptedActions(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, recovered)
	pending, err := st.GetPendingResults(ctx)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	wantDigest, err := contract.ActionDigest(executed)
	require.NoError(t, err)
	require.Equal(t, wantDigest, pending[0].ActionResult.GetActionDigest())
	require.Equal(t, pb.ExecutionStatus_EXECUTION_STATUS_INDETERMINATE, pending[0].ActionResult.GetStatus())
}

func TestInterruptedRetiredActionProducesOneResultAndDeletesWork(t *testing.T) {
	ctx := context.Background()
	st, err := New(t.TempDir())
	require.NoError(t, err)
	defer st.Close()
	action := testAction("01K00000000000000000000041", "true")
	require.NoError(t, st.ReconcilePolicy(ctx, testPolicy(action)))
	due, err := st.GetDueScheduledWork(ctx)
	require.NoError(t, err)
	require.NoError(t, st.BeginActionRun(ctx, &due[0], time.Now()))
	require.NoError(t, st.ReconcilePolicy(ctx, testPolicy()))
	recovered, err := st.RecoverInterruptedActions(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, recovered)
	pending, err := st.GetPendingResults(ctx)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	require.Equal(t, pb.ExecutionStatus_EXECUTION_STATUS_INDETERMINATE, pending[0].ActionResult.GetStatus())
	rows, err := st.queries.ListAllWork(ctx)
	require.NoError(t, err)
	require.Empty(t, rows)
}
