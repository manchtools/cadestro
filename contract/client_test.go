package contract

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"google.golang.org/protobuf/types/known/durationpb"
)

func TestBootstrapHTTPClient_Bounded(t *testing.T) {
	c := bootstrapHTTPClient()
	if c.Timeout == 0 {
		t.Error("bootstrap client has no Timeout (a hung control endpoint could wedge enrollment)")
	}
	tr, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport is %T, want *http.Transport", c.Transport)
	}
	if tr.TLSClientConfig == nil {
		t.Fatal("bootstrap transport has no TLSClientConfig")
	}
	if tr.TLSClientConfig.MinVersion != tls.VersionTLS13 {
		t.Errorf("TLS MinVersion = %#x, want TLS 1.3 (%#x)", tr.TLSClientConfig.MinVersion, tls.VersionTLS13)
	}
}

type fakeTerminalHandler struct {
	startCalls  []*cadestrov1.TerminalStart
	inputCalls  []*cadestrov1.TerminalInput
	resizeCalls []*cadestrov1.TerminalResize
	stopCalls   []*cadestrov1.TerminalStop

	startErr  error
	inputErr  error
	resizeErr error
	stopErr   error
}

func (h *fakeTerminalHandler) OnWelcome(ctx context.Context, w *cadestrov1.Welcome) error {
	return nil
}
func (h *fakeTerminalHandler) OnQuery(ctx context.Context, q *cadestrov1.OSQuery) (*cadestrov1.OSQueryResult, error) {
	return nil, nil
}
func (h *fakeTerminalHandler) OnError(ctx context.Context, e *cadestrov1.Error) error { return nil }

func (h *fakeTerminalHandler) OnTerminalStart(ctx context.Context, req *cadestrov1.TerminalStart) error {
	h.startCalls = append(h.startCalls, req)
	return h.startErr
}
func (h *fakeTerminalHandler) OnTerminalInput(ctx context.Context, req *cadestrov1.TerminalInput) error {
	h.inputCalls = append(h.inputCalls, req)
	return h.inputErr
}
func (h *fakeTerminalHandler) OnTerminalResize(ctx context.Context, req *cadestrov1.TerminalResize) error {
	h.resizeCalls = append(h.resizeCalls, req)
	return h.resizeErr
}
func (h *fakeTerminalHandler) OnTerminalStop(ctx context.Context, req *cadestrov1.TerminalStop) error {
	h.stopCalls = append(h.stopCalls, req)
	return h.stopErr
}

type fakeBareHandler struct{}

func (fakeBareHandler) OnWelcome(ctx context.Context, w *cadestrov1.Welcome) error { return nil }
func (fakeBareHandler) OnQuery(ctx context.Context, q *cadestrov1.OSQuery) (*cadestrov1.OSQueryResult, error) {
	return nil, nil
}
func (fakeBareHandler) OnError(ctx context.Context, e *cadestrov1.Error) error { return nil }

func newTestClient() *Client {
	return NewClient("http://localhost:0")
}

func makeTerminalMsg(name string) *cadestrov1.ServerMessage {
	msg := &cadestrov1.ServerMessage{Id: &cadestrov1.MessageId{Value: NewULID()}}
	switch name {
	case "TerminalStart":
		msg.Payload = &cadestrov1.ServerMessage_TerminalStart{
			TerminalStart: &cadestrov1.TerminalStart{
				SessionId: &cadestrov1.SessionId{Value: "01ARZ3NDEKTSV4RRFFQ69G5FAV"},
				TtyUser:   "cadestro-tty-test",
				Cols:      80,
				Rows:      24,
			},
		}
	case "TerminalInput":
		msg.Payload = &cadestrov1.ServerMessage_TerminalInput{
			TerminalInput: &cadestrov1.TerminalInput{
				SessionId: &cadestrov1.SessionId{Value: "01ARZ3NDEKTSV4RRFFQ69G5FAV"},
				Data:      []byte("ls -la\n"),
			},
		}
	case "TerminalResize":
		msg.Payload = &cadestrov1.ServerMessage_TerminalResize{
			TerminalResize: &cadestrov1.TerminalResize{
				SessionId: &cadestrov1.SessionId{Value: "01ARZ3NDEKTSV4RRFFQ69G5FAV"},
				Cols:      120,
				Rows:      40,
			},
		}
	case "TerminalStop":
		msg.Payload = &cadestrov1.ServerMessage_TerminalStop{
			TerminalStop: &cadestrov1.TerminalStop{
				SessionId: &cadestrov1.SessionId{Value: "01ARZ3NDEKTSV4RRFFQ69G5FAV"},
				Reason:    "admin terminate",
			},
		}
	}
	return msg
}

func TestDispatch_Terminal_Routing(t *testing.T) {
	cases := []struct {
		name   string
		assert func(t *testing.T, h *fakeTerminalHandler)
	}{
		{
			name: "TerminalStart",
			assert: func(t *testing.T, h *fakeTerminalHandler) {
				t.Helper()
				if len(h.startCalls) != 1 {
					t.Fatalf("OnTerminalStart calls = %d, want 1", len(h.startCalls))
				}
				if h.startCalls[0].GetSessionId().GetValue() != "01ARZ3NDEKTSV4RRFFQ69G5FAV" {
					t.Errorf("session_id = %q, want 01ARZ3NDEKTSV4RRFFQ69G5FAV", h.startCalls[0].GetSessionId().GetValue())
				}
				if h.startCalls[0].TtyUser != "cadestro-tty-test" {
					t.Errorf("tty_user = %q, want cadestro-tty-test", h.startCalls[0].TtyUser)
				}
			},
		},
		{
			name: "TerminalInput",
			assert: func(t *testing.T, h *fakeTerminalHandler) {
				t.Helper()
				if len(h.inputCalls) != 1 {
					t.Fatalf("OnTerminalInput calls = %d, want 1", len(h.inputCalls))
				}
				if string(h.inputCalls[0].Data) != "ls -la\n" {
					t.Errorf("data = %q, want %q", h.inputCalls[0].Data, "ls -la\n")
				}
			},
		},
		{
			name: "TerminalResize",
			assert: func(t *testing.T, h *fakeTerminalHandler) {
				t.Helper()
				if len(h.resizeCalls) != 1 {
					t.Fatalf("OnTerminalResize calls = %d, want 1", len(h.resizeCalls))
				}
				if h.resizeCalls[0].Cols != 120 || h.resizeCalls[0].Rows != 40 {
					t.Errorf("size = %dx%d, want 120x40", h.resizeCalls[0].Cols, h.resizeCalls[0].Rows)
				}
			},
		},
		{
			name: "TerminalStop",
			assert: func(t *testing.T, h *fakeTerminalHandler) {
				t.Helper()
				if len(h.stopCalls) != 1 {
					t.Fatalf("OnTerminalStop calls = %d, want 1", len(h.stopCalls))
				}
				if h.stopCalls[0].Reason != "admin terminate" {
					t.Errorf("reason = %q, want admin terminate", h.stopCalls[0].Reason)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newTestClient()
			h := &fakeTerminalHandler{}
			if err := c.dispatchServerMessage(context.Background(), makeTerminalMsg(tc.name), h); err != nil {
				t.Fatalf("dispatch: %v", err)
			}
			tc.assert(t, h)
		})
	}
}

func TestDispatch_Terminal_HandlerErrorPropagates(t *testing.T) {
	cases := []struct {
		name    string
		setErr  func(h *fakeTerminalHandler, want error)
		wantSub string
	}{
		{
			name:    "TerminalStart",
			setErr:  func(h *fakeTerminalHandler, want error) { h.startErr = want },
			wantSub: "handle terminal start",
		},
		{
			name:    "TerminalInput",
			setErr:  func(h *fakeTerminalHandler, want error) { h.inputErr = want },
			wantSub: "handle terminal input",
		},
		{
			name:    "TerminalResize",
			setErr:  func(h *fakeTerminalHandler, want error) { h.resizeErr = want },
			wantSub: "handle terminal resize",
		},
		{
			name:    "TerminalStop",
			setErr:  func(h *fakeTerminalHandler, want error) { h.stopErr = want },
			wantSub: "handle terminal stop",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newTestClient()
			want := errors.New("handler refused")
			h := &fakeTerminalHandler{}
			tc.setErr(h, want)

			err := c.dispatchServerMessage(context.Background(), makeTerminalMsg(tc.name), h)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !errors.Is(err, want) {
				t.Errorf("expected errors.Is(err, want) = true, got err = %v", err)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("err.Error() = %q, want substring %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestDispatch_Terminal_NoHandler_DropsSilently(t *testing.T) {
	c := newTestClient()
	bare := fakeBareHandler{}

	cases := []*cadestrov1.ServerMessage{
		{Id: &cadestrov1.MessageId{Value: NewULID()}, Payload: &cadestrov1.ServerMessage_TerminalStart{TerminalStart: &cadestrov1.TerminalStart{SessionId: &cadestrov1.SessionId{Value: "01"}}}},
		{Id: &cadestrov1.MessageId{Value: NewULID()}, Payload: &cadestrov1.ServerMessage_TerminalInput{TerminalInput: &cadestrov1.TerminalInput{SessionId: &cadestrov1.SessionId{Value: "01"}}}},
		{Id: &cadestrov1.MessageId{Value: NewULID()}, Payload: &cadestrov1.ServerMessage_TerminalResize{TerminalResize: &cadestrov1.TerminalResize{SessionId: &cadestrov1.SessionId{Value: "01"}}}},
		{Id: &cadestrov1.MessageId{Value: NewULID()}, Payload: &cadestrov1.ServerMessage_TerminalStop{TerminalStop: &cadestrov1.TerminalStop{SessionId: &cadestrov1.SessionId{Value: "01"}}}},
	}
	for _, msg := range cases {
		if err := c.dispatchServerMessage(context.Background(), msg, bare); err != nil {
			t.Errorf("dispatch %T: unexpected error: %v", msg.Payload, err)
		}
	}
}

func TestDispatch_UnknownPayload_DropsSilently(t *testing.T) {
	c := newTestClient()
	h := &fakeTerminalHandler{}

	msg := &cadestrov1.ServerMessage{Id: &cadestrov1.MessageId{Value: NewULID()}}
	if err := c.dispatchServerMessage(context.Background(), msg, h); err != nil {
		t.Errorf("dispatch unknown payload: unexpected error: %v", err)
	}

	if len(h.startCalls)+len(h.inputCalls)+len(h.resizeCalls)+len(h.stopCalls) != 0 {
		t.Errorf("unknown payload should not invoke any handler method")
	}
}

var _ TerminalHandler = (*fakeTerminalHandler)(nil)
var _ StreamHandler = fakeBareHandler{}

func TestApplyWelcomeHeartbeat_ClampsAndPushes(t *testing.T) {
	cases := []struct {
		name  string
		input time.Duration
		want  time.Duration
	}{
		{"within range", 45 * time.Second, 45 * time.Second},
		{"min edge", MinHeartbeatInterval, MinHeartbeatInterval},
		{"max edge", MaxHeartbeatInterval, MaxHeartbeatInterval},
		{"below min clamps up", 1 * time.Second, MinHeartbeatInterval},
		{"above max clamps down", 10 * time.Minute, MaxHeartbeatInterval},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newTestClient()

			hb := make(chan time.Duration, 1)
			c.mu.Lock()
			c.heartbeatUpdate = hb
			c.mu.Unlock()

			c.applyWelcomeHeartbeat(&cadestrov1.Welcome{
				HeartbeatInterval: durationpb.New(tc.input),
			})

			select {
			case got := <-hb:
				if got != tc.want {
					t.Errorf("interval = %v, want %v", got, tc.want)
				}
			default:
				t.Fatal("no interval pushed to heartbeat channel")
			}
		})
	}
}

func TestApplyWelcomeHeartbeat_NoOpCases(t *testing.T) {
	cases := []struct {
		name string
		w    *cadestrov1.Welcome
	}{
		{"nil welcome", nil},
		{"unset field", &cadestrov1.Welcome{}},
		{"zero duration", &cadestrov1.Welcome{HeartbeatInterval: durationpb.New(0)}},
		{"negative duration", &cadestrov1.Welcome{HeartbeatInterval: durationpb.New(-5 * time.Second)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newTestClient()
			hb := make(chan time.Duration, 1)
			c.mu.Lock()
			c.heartbeatUpdate = hb
			c.mu.Unlock()

			c.applyWelcomeHeartbeat(tc.w)

			select {
			case got := <-hb:
				t.Errorf("expected no push, got %v", got)
			default:
			}
		})
	}
}

func TestApplyWelcomeHeartbeat_NoRunActive(t *testing.T) {
	c := newTestClient()

	c.applyWelcomeHeartbeat(&cadestrov1.Welcome{
		HeartbeatInterval: durationpb.New(42 * time.Second),
	})

}

func TestApplyWelcomeHeartbeat_LatestWins(t *testing.T) {
	c := newTestClient()
	hb := make(chan time.Duration, 1)
	c.mu.Lock()
	c.heartbeatUpdate = hb
	c.mu.Unlock()

	c.applyWelcomeHeartbeat(&cadestrov1.Welcome{HeartbeatInterval: durationpb.New(10 * time.Second)})
	c.applyWelcomeHeartbeat(&cadestrov1.Welcome{HeartbeatInterval: durationpb.New(45 * time.Second)})

	got := <-hb
	if got != 45*time.Second {
		t.Errorf("interval = %v, want 45s", got)
	}
	select {
	case extra := <-hb:
		t.Errorf("channel should be drained, got extra %v", extra)
	default:
	}
}

func TestDispatch_Welcome_AppliesHeartbeatAndHandler(t *testing.T) {
	c := newTestClient()
	hb := make(chan time.Duration, 1)
	c.mu.Lock()
	c.heartbeatUpdate = hb
	c.mu.Unlock()

	rec := &recordingWelcomeHandler{}
	msg := &cadestrov1.ServerMessage{
		Id: &cadestrov1.MessageId{Value: NewULID()},
		Payload: &cadestrov1.ServerMessage_Welcome{Welcome: &cadestrov1.Welcome{
			ServerVersion:     "test",
			HeartbeatInterval: durationpb.New(60 * time.Second),
		}},
	}
	if err := c.dispatchServerMessage(context.Background(), msg, rec); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if !rec.called {
		t.Error("OnWelcome was not called")
	}
	select {
	case got := <-hb:
		if got != 60*time.Second {
			t.Errorf("interval = %v, want 60s", got)
		}
	default:
		t.Fatal("heartbeat update not pushed")
	}
}

type recordingWelcomeHandler struct {
	called bool
}

func (h *recordingWelcomeHandler) OnWelcome(ctx context.Context, w *cadestrov1.Welcome) error {
	h.called = true
	return nil
}
func (h *recordingWelcomeHandler) OnQuery(ctx context.Context, q *cadestrov1.OSQuery) (*cadestrov1.OSQueryResult, error) {
	return nil, nil
}
func (h *recordingWelcomeHandler) OnError(ctx context.Context, e *cadestrov1.Error) error { return nil }
