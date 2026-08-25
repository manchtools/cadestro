package scheduler

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/manchtools/cadestro/agent/internal/store"
	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

type recordingExecutor struct {
	executed []string
	status   map[string]pb.ExecutionStatus
}

func (e *recordingExecutor) ExecuteAction(_ context.Context, action *pb.Action) *pb.ActionResult {
	id := action.GetId().GetValue()
	e.executed = append(e.executed, id)
	status := e.status[id]
	if status == pb.ExecutionStatus_EXECUTION_STATUS_UNSPECIFIED {
		status = pb.ExecutionStatus_EXECUTION_STATUS_SUCCESS
	}
	return &pb.ActionResult{ActionId: action.GetId(), Status: status, CompletedAt: timestamppb.Now()}
}

func (*recordingExecutor) ResetUpdateCycle() {}

func scheduledManifest(onFailure pb.OnFailure) *pb.Manifest {
	return &pb.Manifest{
		ManifestId: &pb.ManifestId{Value: "01K00000000000000000000012"},
		Schedule:   &pb.ActionSchedule{RunOnAssign: true, IntervalHours: 8},
		Occurrences: []*pb.ManifestOccurrence{
			{OccurrenceId: &pb.OccurrenceId{Value: "01K00000000000000000000013"}, OnFailure: onFailure, Action: &pb.Action{Id: &pb.ActionId{Value: "01K00000000000000000000014"}, Type: pb.ActionType_ACTION_TYPE_PACKAGE}},
			{OccurrenceId: &pb.OccurrenceId{Value: "01K00000000000000000000015"}, Action: &pb.Action{Id: &pb.ActionId{Value: "01K00000000000000000000016"}, Type: pb.ActionType_ACTION_TYPE_SERVICE}},
		},
	}
}
func TestManifestRunsInOrderAndReplayDoesNotDoubleExecute(t *testing.T) {
	st, err := store.New(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, st.Close()) })
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	st.SetClockForTest(func() time.Time { return now })
	exec := &recordingExecutor{status: map[string]pb.ExecutionStatus{}}
	sched := New(context.Background(), st, exec, slog.New(slog.NewTextHandler(io.Discard, nil)))
	sched.now = func() time.Time { return now }
	manifest := scheduledManifest(pb.OnFailure_ON_FAILURE_CONTINUE)

	require.NoError(t, st.ReconcilePolicy(context.Background(), &pb.DesiredPolicy{Revision: manifest.GetManifestId().GetValue(), Manifests: []*pb.Manifest{manifest}}))

	sched.runDue(context.Background())
	require.Equal(t, []string{
		"01K00000000000000000000014",
		"01K00000000000000000000016",
	}, exec.executed)
	sched.runDue(context.Background())
	require.Len(t, exec.executed, 2, "the transport replay must not create another due run")

	pending, err := st.GetPendingResults(context.Background())
	require.NoError(t, err)
	require.Len(t, pending, 3)
	require.NotEmpty(t, pending[0].ActionResult.GetRunId().GetValue())
	require.Equal(t, manifest.GetOccurrences()[0].GetOccurrenceId().GetValue(), pending[0].ActionResult.GetOccurrenceId().GetValue())
	require.Equal(t, pb.ExecutionStatus_EXECUTION_STATUS_SUCCESS, pending[2].ManifestResult.GetStatus())
}

func TestManifestStopPolicyRecordsRemainingOccurrenceAsSkipped(t *testing.T) {
	st, err := store.New(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, st.Close()) })
	manifest := scheduledManifest(pb.OnFailure_ON_FAILURE_STOP)
	firstID := manifest.GetOccurrences()[0].GetAction().GetId().GetValue()
	exec := &recordingExecutor{status: map[string]pb.ExecutionStatus{firstID: pb.ExecutionStatus_EXECUTION_STATUS_FAILED}}
	sched := New(context.Background(), st, exec, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, st.ReconcilePolicy(context.Background(), &pb.DesiredPolicy{Revision: manifest.GetManifestId().GetValue(), Manifests: []*pb.Manifest{manifest}}))
	sched.runDue(context.Background())
	require.Equal(t, []string{firstID}, exec.executed)

	pending, err := st.GetPendingResults(context.Background())
	require.NoError(t, err)
	require.Len(t, pending, 3)
	require.Equal(t, pb.ExecutionStatus_EXECUTION_STATUS_SKIPPED, pending[1].ActionResult.GetStatus())
	require.Equal(t, pb.ExecutionStatus_EXECUTION_STATUS_FAILED, pending[2].ManifestResult.GetStatus())
}

func TestSkipIfUnchangedSuppressesRepeatedActionOutputButStillExecutes(t *testing.T) {
	st, err := store.New(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, st.Close()) })
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	st.SetClockForTest(func() time.Time { return now })
	manifest := scheduledManifest(pb.OnFailure_ON_FAILURE_CONTINUE)
	manifest.Schedule.SkipIfUnchanged = true
	exec := &recordingExecutor{status: map[string]pb.ExecutionStatus{}}
	sched := New(context.Background(), st, exec, slog.New(slog.NewTextHandler(io.Discard, nil)))
	sched.now = func() time.Time { return now }
	require.NoError(t, st.ReconcilePolicy(context.Background(), &pb.DesiredPolicy{Revision: manifest.GetManifestId().GetValue(), Manifests: []*pb.Manifest{manifest}}))

	sched.runDue(context.Background())
	now = now.Add(8 * time.Hour)
	sched.runDue(context.Background())
	require.Len(t, exec.executed, 4, "deduplication must not suppress reconciliation")

	pending, err := st.GetPendingResults(context.Background())
	require.NoError(t, err)
	require.Len(t, pending, 4, "the repeated action outputs are suppressed; each manifest result remains")
	actionResults := 0
	manifestResults := 0
	for _, result := range pending {
		if result.ActionResult != nil {
			actionResults++
		}
		if result.ManifestResult != nil {
			manifestResults++
		}
	}
	require.Equal(t, 2, actionResults)
	require.Equal(t, 2, manifestResults)
}
