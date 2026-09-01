package scheduler

import (
	"context"
	"github.com/manchtools/cadestro/agent/internal/store"
	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/stretchr/testify/require"
	"io"
	"log/slog"
	"testing"
	"time"
)

type testExecutor struct{}

func (testExecutor) ExecuteAction(_ context.Context, action *pb.Action) *pb.ActionResult {
	return &pb.ActionResult{ActionId: action.Id, Status: pb.ExecutionStatus_EXECUTION_STATUS_SUCCESS}
}
func TestActionResultSignalsWakeWithoutPayload(t *testing.T) {
	st, err := store.New(t.TempDir())
	require.NoError(t, err)
	defer st.Close()
	s := New(st, testExecutor{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	s.now = time.Now
	action := &pb.Action{Id: &pb.ActionId{Value: "01K00000000000000000000031"}, Params: &pb.Action_Shell{Shell: &pb.ShellActionParams{Script: "true"}}}
	require.NoError(t, st.ReconcilePolicy(context.Background(), &pb.DesiredPolicy{Revision: &pb.PolicyRevisionId{Value: "01K00000000000000000000032"}, Actions: []*pb.Action{action}}))
	select {
	case <-s.ResultsReady():
		t.Fatal("policy reconciliation signaled a result before execution")
	default:
	}
	due, err := st.GetDueScheduledWork(context.Background())
	require.NoError(t, err)
	require.Len(t, due, 1)
	s.executeAction(context.Background(), due[0])
	select {
	case <-s.ResultsReady():
	case <-time.After(time.Second):
		t.Fatal("scheduler did not signal wake")
	}
	pending, err := st.GetPendingResults(context.Background())
	require.NoError(t, err)
	require.Len(t, pending, 1)
}
