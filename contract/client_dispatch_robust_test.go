package contract

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

type panicStreamingHandler struct {
	ran         chan struct{}
	panicInv    bool
	panicRevoke bool
	mu          sync.Mutex
	closed      bool
}

func (h *panicStreamingHandler) signalRan() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.closed {
		close(h.ran)
		h.closed = true
	}
}

func (h *panicStreamingHandler) OnWelcome(ctx context.Context, w *cadestrov1.Welcome) error {
	return nil
}
func (h *panicStreamingHandler) OnQuery(ctx context.Context, q *cadestrov1.OSQuery) (*cadestrov1.OSQueryResult, error) {
	return nil, nil
}
func (h *panicStreamingHandler) OnError(ctx context.Context, e *cadestrov1.Error) error { return nil }

func (h *panicStreamingHandler) CollectInventory(ctx context.Context) *cadestrov1.DeviceInventory {
	return nil
}
func (h *panicStreamingHandler) OnRequestInventory(ctx context.Context, req *cadestrov1.RequestInventory) *cadestrov1.DeviceInventory {
	if h.panicInv {
		h.signalRan()
		panic("boom: inventory handler exploded")
	}
	return nil
}
func (h *panicStreamingHandler) OnRevokeLuksDeviceKey(ctx context.Context, req *cadestrov1.RevokeLuksDeviceKey) (bool, string) {
	if h.panicRevoke {
		h.signalRan()
		panic("boom: revoke handler exploded")
	}
	return false, ""
}

func TestDispatch_HandlerPanic_Recovered_LoopSurvives(t *testing.T) {
	t.Run("fan-out leg: inventory goroutine panic must not crash the process", func(t *testing.T) {
		c := NewClient("https://gw.invalid", WithAuth("01HZZZZZZZZZZZZZZZZZZZZZZZZ", ""))
		h := &panicStreamingHandler{ran: make(chan struct{}), panicInv: true}
		msg := &cadestrov1.ServerMessage{
			Id: &cadestrov1.MessageId{Value: NewULID()},
			Payload: &cadestrov1.ServerMessage_RequestInventory{
				RequestInventory: &cadestrov1.RequestInventory{QueryId: &cadestrov1.QueryId{Value: "01HQ0000000000000000000000"}},
			},
		}
		if err := c.dispatchServerMessage(context.Background(), msg, h); err != nil {
			t.Fatalf("dispatch returned error: %v", err)
		}

		select {
		case <-h.ran:
		case <-time.After(2 * time.Second):
			t.Fatal("inventory handler never ran")
		}

		time.Sleep(50 * time.Millisecond)
	})

	t.Run("fan-out leg: luks-revoke goroutine panic must not crash the process", func(t *testing.T) {
		c := NewClient("https://gw.invalid", WithAuth("01HZZZZZZZZZZZZZZZZZZZZZZZZ", ""))
		h := &panicStreamingHandler{ran: make(chan struct{}), panicRevoke: true}
		msg := &cadestrov1.ServerMessage{
			Id: &cadestrov1.MessageId{Value: NewULID()},
			Payload: &cadestrov1.ServerMessage_RevokeLuksDeviceKey{
				RevokeLuksDeviceKey: &cadestrov1.RevokeLuksDeviceKey{ActionId: &cadestrov1.ActionId{Value: "01HQ0000000000000000000000"}},
			},
		}
		if err := c.dispatchServerMessage(context.Background(), msg, h); err != nil {
			t.Fatalf("dispatch returned error: %v", err)
		}
		select {
		case <-h.ran:
		case <-time.After(2 * time.Second):
			t.Fatal("revoke handler never ran")
		}
		time.Sleep(50 * time.Millisecond)
	})
}

func TestRun_InboundMessageSizeBounded(t *testing.T) {
	if maxInboundMessageBytes <= 0 {
		t.Fatalf("maxInboundMessageBytes = %d, want a positive bound", maxInboundMessageBytes)
	}

	t.Run("within limit: normal message round-trips", func(t *testing.T) {
		l := newAgentLoopback(t)
		welcomeOnce := func(ctx context.Context, s *connect.BidiStream[cadestrov1.AgentMessage, cadestrov1.ServerMessage]) error {
			if _, err := s.Receive(); err != nil {
				return err
			}
			if err := s.Send(&cadestrov1.ServerMessage{
				Id:      &cadestrov1.MessageId{Value: NewULID()},
				Payload: &cadestrov1.ServerMessage_Welcome{Welcome: &cadestrov1.Welcome{ServerVersion: "test"}},
			}); err != nil {
				return err
			}
			for {
				if _, err := s.Receive(); err != nil {
					return nil
				}
			}
		}
		l.handler.onStream = welcomeOnce

		c := l.newClient(WithAuth("device", "tok"))
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := c.Connect(ctx); err != nil {
			t.Fatalf("Connect: %v", err)
		}
		defer c.Close()
		if err := c.SendHello(ctx, "h", "v"); err != nil {
			t.Fatalf("SendHello: %v", err)
		}
		msg, err := c.Receive(ctx)
		if err != nil {
			t.Fatalf("Receive within limit failed: %v", err)
		}
		if msg.GetWelcome() == nil {
			t.Fatalf("expected Welcome, got %T", msg.Payload)
		}
	})

	t.Run("over limit: oversized inbound frame is refused (resource exhausted)", func(t *testing.T) {
		l := newAgentLoopback(t)
		oversize := func(ctx context.Context, s *connect.BidiStream[cadestrov1.AgentMessage, cadestrov1.ServerMessage]) error {
			if _, err := s.Receive(); err != nil {
				return err
			}

			big := make([]byte, maxInboundMessageBytes+(1<<20))
			for i := range big {
				big[i] = 'a'
			}
			_ = s.Send(&cadestrov1.ServerMessage{
				Id: &cadestrov1.MessageId{Value: NewULID()},
				Payload: &cadestrov1.ServerMessage_Error{
					Error: &cadestrov1.Error{Code: "x", Message: string(big)},
				},
			})
			for {
				if _, err := s.Receive(); err != nil {
					return nil
				}
			}
		}
		l.handler.onStream = oversize

		c := l.newClient(WithAuth("device", "tok"))
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := c.Connect(ctx); err != nil {
			t.Fatalf("Connect: %v", err)
		}
		defer c.Close()
		if err := c.SendHello(ctx, "h", "v"); err != nil {
			t.Fatalf("SendHello: %v", err)
		}
		_, err := c.Receive(ctx)
		if err == nil {
			t.Fatal("oversized inbound frame was accepted; expected a resource-exhausted error")
		}
		var connectErr *connect.Error
		if !errors.As(err, &connectErr) {
			t.Fatalf("want a *connect.Error for the oversized frame, got %T: %v", err, err)
		}
		if connectErr.Code() != connect.CodeResourceExhausted {
			t.Fatalf("oversized frame code = %v, want %v", connectErr.Code(), connect.CodeResourceExhausted)
		}
	})
}
