package contract

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

type noopStreamHandler struct{}

func (noopStreamHandler) OnWelcome(context.Context, *cadestrov1.Welcome) error { return nil }
func (noopStreamHandler) OnQuery(context.Context, *cadestrov1.OSQuery) (*cadestrov1.OSQueryResult, error) {
	return nil, nil
}
func (noopStreamHandler) OnError(context.Context, *cadestrov1.Error) error { return nil }

func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func testSecretBytes() []byte {
	return []byte("secret-bytes")
}

func TestDispatchServerMessage_DeliversEveryPendingResponse(t *testing.T) {
	cases := []struct {
		name    string
		payload func() *cadestrov1.ServerMessage
	}{
		{
			name: "StoreLuksKey",
			payload: func() *cadestrov1.ServerMessage {
				return &cadestrov1.ServerMessage{Payload: &cadestrov1.ServerMessage_StoreLuksKey{
					StoreLuksKey: &cadestrov1.StoreLuksKeyResponse{Success: true},
				}}
			},
		},
		{
			name: "GetLuksKey",
			payload: func() *cadestrov1.ServerMessage {
				return &cadestrov1.ServerMessage{Payload: &cadestrov1.ServerMessage_GetLuksKey{
					GetLuksKey: &cadestrov1.GetLuksKeyResponse{Passphrase: testSecretBytes()},
				}}
			},
		},
		{
			name: "StoreLpsPasswords",
			payload: func() *cadestrov1.ServerMessage {
				return &cadestrov1.ServerMessage{Payload: &cadestrov1.ServerMessage_StoreLpsPasswords{
					StoreLpsPasswords: &cadestrov1.StoreLpsPasswordsResponse{Success: true},
				}}
			},
		},
		{
			name: "ValidateLuksToken",
			payload: func() *cadestrov1.ServerMessage {
				return &cadestrov1.ServerMessage{Payload: &cadestrov1.ServerMessage_ValidateLuksToken{
					ValidateLuksToken: &cadestrov1.ValidateLuksTokenResponse{ActionId: &cadestrov1.ActionId{Value: NewULID()}},
				}}
			},
		},
		{
			name: "SyncState",
			payload: func() *cadestrov1.ServerMessage {
				return &cadestrov1.ServerMessage{Payload: &cadestrov1.ServerMessage_SyncState{
					SyncState: &cadestrov1.SyncState{},
				}}
			},
		},
	}

	if len(cases) == 0 {
		t.Fatal("matches-zero: no pending-response types under test")
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &Client{logger: quietLogger()}
			id := NewULID()
			ch := c.registerPending(id)
			defer c.unregisterPending(id)

			msg := tc.payload()
			msg.Id = &cadestrov1.MessageId{Value: id}

			if err := c.dispatchServerMessage(context.Background(), msg, noopStreamHandler{}); err != nil {
				t.Fatalf("dispatchServerMessage: %v", err)
			}

			select {
			case got := <-ch:
				if got.GetId().GetValue() != id {
					t.Errorf("delivered message id = %q, want %q", got.GetId().GetValue(), id)
				}
			case <-time.After(time.Second):
				t.Fatalf("%s response was never delivered to the waiting caller — "+
					"dispatchServerMessage drops it, so the sender blocks until its context expires",
					tc.name)
			}
		})
	}
}

func TestDispatchCorrelatedResponsesAreRoutedAndValidated(t *testing.T) {
	want := map[string]bool{
		"ServerMessage_Error":             true,
		"ServerMessage_SyncState":         true,
		"ServerMessage_GetLuksKey":        true,
		"ServerMessage_StoreLuksKey":      true,
		"ServerMessage_StoreLpsPasswords": true,
		"ServerMessage_ValidateLuksToken": true,
		"ServerMessage_ResultAck":         true,
	}
	cases := parseDispatchCases(t)
	for wrapper := range want {
		info, ok := cases[wrapper]
		if !ok || !info.deliversPending {
			t.Errorf("correlated response %q has no pending-response route", wrapper)
		}
		if !info.validates {
			t.Errorf("correlated response %q is routed without inbound validation", wrapper)
		}
	}
	for wrapper, info := range cases {
		if info.deliversPending && !info.validates {
			t.Errorf("pending-response arm %q is routed without inbound validation", wrapper)
		}
	}
}

func TestDispatchServerMessage_DeliversCorrelatedErrorToTheWaiter(t *testing.T) {
	c := &Client{logger: quietLogger()}
	id := NewULID()
	ch := c.registerPending(id)
	defer c.unregisterPending(id)

	msg := &cadestrov1.ServerMessage{
		Id: &cadestrov1.MessageId{Value: id},
		Payload: &cadestrov1.ServerMessage_Error{
			Error: &cadestrov1.Error{Message: "failed to store LPS passwords"},
		},
	}

	handler := &recordingErrHandler{}
	if err := c.dispatchServerMessage(context.Background(), msg, handler); err != nil {
		t.Fatalf("dispatchServerMessage: %v", err)
	}

	select {
	case got := <-ch:
		if got.GetError().GetMessage() != "failed to store LPS passwords" {
			t.Errorf("waiter got %+v, want the server's rejection", got)
		}
	default:
		t.Fatal("the correlated error never reached the waiter — the caller blocks until its context expires " +
			"while the server has already answered, stalling the rollback of an irreversible change")
	}

	if handler.calls != 0 {
		t.Errorf("a correlated error also went to OnError (%d calls) — it belongs to its waiter, not the general handler", handler.calls)
	}
}

func TestDispatchServerMessage_UncorrelatedErrorStillReachesTheHandler(t *testing.T) {
	c := &Client{logger: quietLogger()}
	handler := &recordingErrHandler{}

	msg := &cadestrov1.ServerMessage{
		Id: &cadestrov1.MessageId{Value: NewULID()},
		Payload: &cadestrov1.ServerMessage_Error{
			Error: &cadestrov1.Error{Message: "server-originated"},
		},
	}
	if err := c.dispatchServerMessage(context.Background(), msg, handler); err != nil {
		t.Fatalf("dispatchServerMessage: %v", err)
	}
	if handler.calls != 1 {
		t.Errorf("OnError calls = %d, want 1 — an error with no waiter must not be swallowed", handler.calls)
	}
}

type recordingErrHandler struct {
	noopStreamHandler
	calls int
}

func (h *recordingErrHandler) OnError(context.Context, *cadestrov1.Error) error {
	h.calls++
	return nil
}
