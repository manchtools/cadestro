package contract

import (
	"connectrpc.com/connect"
	"context"
	"errors"
	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/contract/gen/go/cadestro/v1/cadestrov1connect"
	"google.golang.org/protobuf/types/known/durationpb"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type heartbeatHandler struct {
	allow              chan struct{}
	hello              chan struct{}
	received           chan *cadestrov1.AgentMessage
	welcome            *cadestrov1.Welcome
	finishAfterWelcome bool
}

func (h *heartbeatHandler) Stream(ctx context.Context, stream *connect.BidiStream[cadestrov1.AgentMessage, cadestrov1.ServerMessage]) error {
	message, err := stream.Receive()
	if err != nil {
		return err
	}
	if _, ok := message.GetPayload().(*cadestrov1.AgentMessage_Hello); !ok {
		return errors.New("expected hello")
	}
	close(h.hello)
	select {
	case <-h.allow:
	case <-ctx.Done():
		return ctx.Err()
	}
	if err := stream.Send(&cadestrov1.ServerMessage{Id: &cadestrov1.MessageId{Value: NewULID()}, Payload: &cadestrov1.ServerMessage_Welcome{Welcome: h.welcome}}); err != nil {
		return err
	}
	if h.finishAfterWelcome {
		return nil
	}
	message, err = stream.Receive()
	if err != nil {
		return err
	}
	h.received <- message
	return nil
}

type heartbeatClientHandler struct{}

func (heartbeatClientHandler) OnWelcome(context.Context, *cadestrov1.Welcome) error { return nil }

func TestRunStartsHeartbeatOnlyAfterWelcome(t *testing.T) {
	service := &heartbeatHandler{allow: make(chan struct{}), hello: make(chan struct{}), received: make(chan *cadestrov1.AgentMessage, 1), welcome: &cadestrov1.Welcome{HeartbeatInterval: durationpb.New(10 * time.Millisecond)}}
	path, handler := cadestrov1connect.NewAgentServiceHandler(service)
	mux := http.NewServeMux()
	mux.Handle(path, handler)
	server := httptest.NewUnstartedServer(mux)
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client := NewClient(server.URL, WithHTTPClient(server.Client()), WithAuth("01K00000000000000000000041", ""))
	run := make(chan error, 1)
	go func() { run <- client.Run(ctx, "host", "version", heartbeatClientHandler{}) }()
	select {
	case <-service.hello:
	case <-time.After(time.Second):
		t.Fatal("hello not received")
	}
	select {
	case <-service.received:
		t.Fatal("heartbeat received before welcome")
	case <-time.After(50 * time.Millisecond):
	}
	close(service.allow)
	select {
	case message := <-service.received:
		if _, ok := message.GetPayload().(*cadestrov1.AgentMessage_Heartbeat); !ok {
			t.Fatalf("payload = %T", message.GetPayload())
		}
	case <-time.After(time.Second):
		t.Fatal("heartbeat not received after welcome")
	}
	cancel()
	select {
	case <-run:
	case <-time.After(time.Second):
		t.Fatal("client did not stop")
	}
}
func TestRunRejectsNonPositiveWelcomeHeartbeat(t *testing.T) {
	service := &heartbeatHandler{allow: make(chan struct{}), hello: make(chan struct{}), received: make(chan *cadestrov1.AgentMessage, 1), welcome: &cadestrov1.Welcome{HeartbeatInterval: durationpb.New(0)}, finishAfterWelcome: true}
	path, handler := cadestrov1connect.NewAgentServiceHandler(service)
	mux := http.NewServeMux()
	mux.Handle(path, handler)
	server := httptest.NewUnstartedServer(mux)
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client := NewClient(server.URL, WithHTTPClient(server.Client()), WithAuth("01K00000000000000000000042", ""))
	run := make(chan error, 1)
	go func() { run <- client.Run(ctx, "host", "version", heartbeatClientHandler{}) }()
	select {
	case <-service.hello:
	case <-time.After(time.Second):
		t.Fatal("hello not received")
	}
	close(service.allow)
	select {
	case err := <-run:
		if err == nil || err.Error() != "welcome heartbeat interval must be positive" {
			t.Fatalf("error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("client did not reject heartbeat interval")
	}
}
