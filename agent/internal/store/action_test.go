package store

import (
	"context"
	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func testAction(id, script string) *pb.Action {
	return &pb.Action{Id: &pb.ActionId{Value: id}, Params: &pb.Action_Shell{Shell: &pb.ShellActionParams{Script: script}}, Schedule: &pb.ActionSchedule{IntervalHours: 1}}
}
func testPolicy(revision string, actions ...*pb.Action) *pb.DesiredPolicy {
	return &pb.DesiredPolicy{Revision: &pb.PolicyRevisionId{Value: revision}, Actions: actions}
}
func TestReconcileActionUpdatesOnlyChangedRows(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	st, err := New(t.TempDir())
	require.NoError(t, err)
	defer st.Close()
	st.now = func() time.Time { return now }
	id := "01K00000000000000000000001"
	a := testAction(id, "true")
	require.NoError(t, st.ReconcilePolicy(ctx, testPolicy("01K00000000000000000000002", a)))
	due, err := st.GetDueScheduledWork(ctx)
	require.NoError(t, err)
	require.Len(t, due, 1)
	require.NoError(t, st.BeginActionRun(ctx, &due[0], now))
	result := &pb.ActionResult{ActionId: a.Id, RunId: &pb.RunId{Value: due[0].RunID}, Status: pb.ExecutionStatus_EXECUTION_STATUS_SUCCESS}
	_, err = st.RecordActionResult(ctx, result)
	require.NoError(t, err)
	a2 := testAction(id, "false")
	now = now.Add(10 * time.Minute)
	require.NoError(t, st.ReconcilePolicy(ctx, testPolicy("01K00000000000000000000003", a2)))
	due, err = st.GetDueScheduledWork(ctx)
	require.NoError(t, err)
	require.Len(t, due, 1)
	require.Equal(t, "false", due[0].Action.GetShell().GetScript())
	next, err := st.queries.GetScheduledWorkNextExecute(ctx, id)
	require.NoError(t, err)
	now = now.Add(time.Minute)
	require.NoError(t, st.ReconcilePolicy(ctx, testPolicy("01K00000000000000000000004", a2)))
	due, err = st.GetDueScheduledWork(ctx)
	require.NoError(t, err)
	require.Len(t, due, 1)
	nextAfter, err := st.queries.GetScheduledWorkNextExecute(ctx, id)
	require.NoError(t, err)
	require.Equal(t, next, nextAfter)
}
func TestActionResultOutboxAckDeletesOne(t *testing.T) {
	ctx := context.Background()
	st, err := New(t.TempDir())
	require.NoError(t, err)
	defer st.Close()
	id := "01K00000000000000000000011"
	a := testAction(id, "true")
	require.NoError(t, st.ReconcilePolicy(ctx, testPolicy("01K00000000000000000000012", a)))
	due, err := st.GetDueScheduledWork(ctx)
	require.NoError(t, err)
	require.NoError(t, st.BeginActionRun(ctx, &due[0], time.Now()))
	seq, err := st.RecordActionResult(ctx, &pb.ActionResult{ActionId: a.Id, RunId: &pb.RunId{Value: due[0].RunID}, Status: pb.ExecutionStatus_EXECUTION_STATUS_SUCCESS})
	require.NoError(t, err)
	pending, err := st.GetPendingResults(ctx)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	require.Equal(t, seq, pending[0].Sequence)
	require.NoError(t, st.DeletePendingResult(ctx, seq))
	pending, err = st.GetPendingResults(ctx)
	require.NoError(t, err)
	require.Empty(t, pending)
}
func TestInterruptedRetiredActionProducesOneResultAndDeletesWork(t *testing.T) {
	ctx := context.Background()
	st, err := New(t.TempDir())
	require.NoError(t, err)
	defer st.Close()
	id := "01K00000000000000000000021"
	a := testAction(id, "true")
	require.NoError(t, st.ReconcilePolicy(ctx, testPolicy("01K00000000000000000000022", a)))
	due, err := st.GetDueScheduledWork(ctx)
	require.NoError(t, err)
	require.NoError(t, st.BeginActionRun(ctx, &due[0], time.Now()))
	require.NoError(t, st.ReconcilePolicy(ctx, testPolicy("01K00000000000000000000023")))
	require.NoError(t, st.RecoverInterruptedActions(ctx))
	pending, err := st.GetPendingResults(ctx)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	require.Equal(t, pb.ExecutionStatus_EXECUTION_STATUS_INDETERMINATE, pending[0].ActionResult.GetStatus())
	rows, err := st.queries.ListAllWork(ctx)
	require.NoError(t, err)
	require.Empty(t, rows)
}
