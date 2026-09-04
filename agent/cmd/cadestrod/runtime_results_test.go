package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/manchtools/cadestro/agent/internal/scheduler"
	"github.com/manchtools/cadestro/agent/internal/store"
	contract "github.com/manchtools/cadestro/contract"
	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/contract/gen/go/cadestro/v1/cadestrov1connect"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type resultAckHandler struct {
	received chan string
}

func (handler resultAckHandler) Stream(ctx context.Context, stream *connect.BidiStream[pb.AgentMessage, pb.ServerMessage]) error {
	if _, err := stream.Receive(); err != nil {
		return err
	}
	if err := stream.Send(&pb.ServerMessage{Id: &pb.MessageId{Value: "01K00000000000000000000061"}, Payload: &pb.ServerMessage_Welcome{Welcome: &pb.Welcome{HeartbeatInterval: durationpb.New(time.Hour)}}}); err != nil {
		return err
	}
	for index := range 2 {
		message, err := stream.Receive()
		if err != nil {
			return err
		}
		handler.received <- message.GetActionResult().GetRunId().GetValue()
		code := pb.ResultAckCode_RESULT_ACK_CODE_ACCEPTED
		if index == 0 {
			code = pb.ResultAckCode_RESULT_ACK_CODE_REJECTED
		}
		if err := stream.Send(&pb.ServerMessage{Id: message.Id, Payload: &pb.ServerMessage_ResultAck{ResultAck: &pb.ResultAck{Code: code}}}); err != nil {
			return err
		}
	}
	<-ctx.Done()
	return ctx.Err()
}

func TestSyncPendingResultsDropsRejectedAndContinues(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	st, err := store.New(t.TempDir())
	require.NoError(t, err)
	defer st.Close()
	actions := []*pb.Action{
		{Id: &pb.ActionId{Value: "01K00000000000000000000062"}, Params: &pb.Action_Update{Update: &pb.UpdateActionParams{}}},
		{Id: &pb.ActionId{Value: "01K00000000000000000000063"}, Params: &pb.Action_Update{Update: &pb.UpdateActionParams{}}},
	}
	require.NoError(t, st.ReconcilePolicy(ctx, &pb.DesiredPolicy{Actions: actions}))
	due, err := st.GetDueScheduledWork(ctx)
	require.NoError(t, err)
	require.Len(t, due, 2)
	for index := range due {
		require.NoError(t, st.BeginActionRun(ctx, &due[index], time.Now()))
		digest, err := contract.ActionDigest(due[index].Action)
		require.NoError(t, err)
		_, err = st.RecordActionResult(ctx, &pb.ActionResult{ActionId: due[index].Action.Id, RunId: &pb.RunId{Value: due[index].RunID}, Status: pb.ExecutionStatus_EXECUTION_STATUS_SUCCESS, CompletedAt: timestamppb.Now(), ActionDigest: digest})
		require.NoError(t, err)
	}
	service := resultAckHandler{received: make(chan string, 2)}
	path, handler := cadestrov1connect.NewAgentServiceHandler(service)
	mux := http.NewServeMux()
	mux.Handle(path, handler)
	server := httptest.NewUnstartedServer(mux)
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	client := contract.NewClient(server.URL, contract.WithHTTPClient(server.Client()), contract.WithDeviceID("01K00000000000000000000064"), contract.WithLogger(logger))
	ready := make(chan struct{}, 1)
	run := make(chan error, 1)
	go func() { run <- client.Run(ctx, "host", "version", ready) }()
	select {
	case <-ready:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	scheduled := scheduler.New(st, nil, logger)
	syncPendingResults(ctx, scheduled, client, logger)
	pending, err := st.GetPendingResults(ctx)
	require.NoError(t, err)
	require.Empty(t, pending)
	for _, expected := range due {
		select {
		case received := <-service.received:
			require.Equal(t, expected.RunID, received)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	cancel()
	select {
	case <-run:
	case <-time.After(time.Second):
		t.Fatal("client did not stop")
	}
}
