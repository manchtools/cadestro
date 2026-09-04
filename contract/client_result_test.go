package contract

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/contract/gen/go/cadestro/v1/cadestrov1connect"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type resultHandler struct {
	response func(*cadestrov1.AgentMessage) *cadestrov1.ServerMessage
}

func (handler resultHandler) Stream(ctx context.Context, stream *connect.BidiStream[cadestrov1.AgentMessage, cadestrov1.ServerMessage]) error {
	if _, err := stream.Receive(); err != nil {
		return err
	}
	if err := stream.Send(&cadestrov1.ServerMessage{Id: &cadestrov1.MessageId{Value: "01K00000000000000000000051"}, Payload: &cadestrov1.ServerMessage_Welcome{Welcome: &cadestrov1.Welcome{HeartbeatInterval: durationpb.New(time.Hour)}}}); err != nil {
		return err
	}
	message, err := stream.Receive()
	if err != nil {
		return err
	}
	if err := stream.Send(handler.response(message)); err != nil {
		return err
	}
	<-ctx.Done()
	return ctx.Err()
}

func testResultClient(t *testing.T, response func(*cadestrov1.AgentMessage) *cadestrov1.ServerMessage) (*Client, context.Context, context.CancelFunc) {
	t.Helper()
	path, handler := cadestrov1connect.NewAgentServiceHandler(resultHandler{response: response})
	mux := http.NewServeMux()
	mux.Handle(path, handler)
	server := httptest.NewUnstartedServer(mux)
	server.EnableHTTP2 = true
	server.StartTLS()
	t.Cleanup(server.Close)
	ctx, cancel := context.WithCancel(context.Background())
	client := NewClient(server.URL, WithHTTPClient(server.Client()), WithDeviceID("01K00000000000000000000052"))
	ready := make(chan struct{}, 1)
	run := make(chan error, 1)
	go func() { run <- client.Run(ctx, "host", "version", ready) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-run:
		case <-time.After(time.Second):
			t.Error("client did not stop")
		}
	})
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("client did not become ready")
	}
	return client, ctx, cancel
}

func TestSendActionResultClassifiesAcknowledgement(t *testing.T) {
	for _, test := range []struct {
		name     string
		response func(*cadestrov1.AgentMessage) *cadestrov1.ServerMessage
		want     error
	}{
		{name: "accepted", response: resultAck(cadestrov1.ResultAckCode_RESULT_ACK_CODE_ACCEPTED)},
		{name: "rejected", response: resultAck(cadestrov1.ResultAckCode_RESULT_ACK_CODE_REJECTED), want: ErrResultRejected},
		{name: "wrong payload", response: func(message *cadestrov1.AgentMessage) *cadestrov1.ServerMessage {
			return &cadestrov1.ServerMessage{Id: message.Id, Payload: &cadestrov1.ServerMessage_DesiredPolicy{DesiredPolicy: &cadestrov1.DesiredPolicy{RefreshIntervalMinutes: 5}}}
		}, want: errors.New("protocol")},
		{name: "unknown code", response: resultAck(cadestrov1.ResultAckCode(99)), want: errors.New("protocol")},
	} {
		t.Run(test.name, func(t *testing.T) {
			client, sessionCtx, _ := testResultClient(t, test.response)
			ctx, cancel := context.WithTimeout(sessionCtx, time.Second)
			defer cancel()
			action := &cadestrov1.Action{Id: &cadestrov1.ActionId{Value: "01K00000000000000000000053"}, Params: &cadestrov1.Action_Update{Update: &cadestrov1.UpdateActionParams{}}}
			digest, err := ActionDigest(action)
			if err != nil {
				t.Fatal(err)
			}
			err = client.SendActionResult(ctx, &cadestrov1.ActionResult{ActionId: action.Id, RunId: &cadestrov1.RunId{Value: "01K00000000000000000000054"}, Status: cadestrov1.ExecutionStatus_EXECUTION_STATUS_SUCCESS, CompletedAt: timestamppb.Now(), ActionDigest: digest})
			switch test.name {
			case "accepted":
				if err != nil {
					t.Fatal(err)
				}
			case "rejected":
				if !errors.Is(err, test.want) {
					t.Fatalf("error = %v", err)
				}
			default:
				if err == nil || errors.Is(err, ErrResultRejected) {
					t.Fatalf("error = %v", err)
				}
			}
		})
	}
}

func TestDeliverPendingConsumesCorrelationOnce(t *testing.T) {
	client := &Client{pending: make(map[string]chan *cadestrov1.ServerMessage)}
	id := "01K00000000000000000000057"
	client.registerPending(id)
	message := &cadestrov1.ServerMessage{Id: &cadestrov1.MessageId{Value: id}}
	if !client.deliverPending(message) {
		t.Fatal("first response was not delivered")
	}
	if client.deliverPending(message) {
		t.Fatal("duplicate response reused consumed correlation")
	}
}

func resultAck(code cadestrov1.ResultAckCode) func(*cadestrov1.AgentMessage) *cadestrov1.ServerMessage {
	return func(message *cadestrov1.AgentMessage) *cadestrov1.ServerMessage {
		return &cadestrov1.ServerMessage{Id: message.Id, Payload: &cadestrov1.ServerMessage_ResultAck{ResultAck: &cadestrov1.ResultAck{Code: code}}}
	}
}

type channelWriter chan struct{}

func (writer channelWriter) Write(payload []byte) (int, error) {
	select {
	case writer <- struct{}{}:
	default:
	}
	return len(payload), nil
}

type welcomeThenCloseHandler struct{}

func (welcomeThenCloseHandler) Stream(_ context.Context, stream *connect.BidiStream[cadestrov1.AgentMessage, cadestrov1.ServerMessage]) error {
	if _, err := stream.Receive(); err != nil {
		return err
	}
	return stream.Send(&cadestrov1.ServerMessage{Id: &cadestrov1.MessageId{Value: "01K00000000000000000000055"}, Payload: &cadestrov1.ServerMessage_Welcome{Welcome: &cadestrov1.Welcome{HeartbeatInterval: durationpb.New(200 * time.Millisecond)}}})
}

func TestRunStopsHeartbeatWhenStreamClosesBeforeParentContext(t *testing.T) {
	path, handler := cadestrov1connect.NewAgentServiceHandler(welcomeThenCloseHandler{})
	mux := http.NewServeMux()
	mux.Handle(path, handler)
	server := httptest.NewUnstartedServer(mux)
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	writes := make(channelWriter, 1)
	client := NewClient(server.URL, WithHTTPClient(server.Client()), WithDeviceID("01K00000000000000000000056"), WithLogger(slog.New(slog.NewTextHandler(writes, &slog.HandlerOptions{Level: slog.LevelDebug}))))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	run := make(chan error, 1)
	go func() { run <- client.Run(ctx, "host", "version", make(chan struct{}, 1)) }()
	select {
	case <-run:
	case <-time.After(time.Second):
		t.Fatal("Run did not return after server closed stream")
	}
	select {
	case <-writes:
		t.Fatal("heartbeat worker logged after Run returned")
	case <-time.After(300 * time.Millisecond):
	}
}
