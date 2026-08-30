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
	executed int
	status   pb.ExecutionStatus
}

func (executor *recordingExecutor) ExecuteAction(_ context.Context, action *pb.Action) *pb.ActionResult {
	executor.executed++
	return &pb.ActionResult{ActionId: action.GetId(), Status: executor.status, CompletedAt: timestamppb.Now()}
}

func (*recordingExecutor) ResetUpdateCycle() {}

func TestDueActionRunsOnceAndQueuesResults(t *testing.T) {
	st, err := store.New(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, st.Close()) })
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	st.SetClockForTest(func() time.Time { return now })
	executor := &recordingExecutor{status: pb.ExecutionStatus_EXECUTION_STATUS_SUCCESS}
	scheduler := New(st, executor, slog.New(slog.NewTextHandler(io.Discard, nil)))
	scheduler.now = func() time.Time { return now }
	manifest := &pb.Manifest{
		ManifestId:   &pb.ManifestId{Value: "01K00000000000000000000012"},
		OccurrenceId: &pb.OccurrenceId{Value: "01K00000000000000000000013"},
		Action:       &pb.Action{Id: &pb.ActionId{Value: "01K00000000000000000000014"}, Params: &pb.Action_Update{Update: &pb.UpdateActionParams{}}},
		Schedule:     &pb.ActionSchedule{RunOnAssign: true, IntervalHours: 8},
	}
	require.NoError(t, st.ReconcilePolicy(context.Background(), &pb.DesiredPolicy{
		Revision: &pb.PolicyRevisionId{Value: "01K00000000000000000000015"}, Manifests: []*pb.Manifest{manifest},
	}))

	scheduler.runDue(context.Background())
	scheduler.runDue(context.Background())
	require.Equal(t, 1, executor.executed)
	pending, err := st.GetPendingResults(context.Background())
	require.NoError(t, err)
	require.Len(t, pending, 2)
	require.Equal(t, pb.ExecutionStatus_EXECUTION_STATUS_SUCCESS, pending[0].ActionResult.GetStatus())
	require.Equal(t, pb.ExecutionStatus_EXECUTION_STATUS_SUCCESS, pending[1].ManifestResult.GetStatus())
}
