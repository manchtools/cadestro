package handler

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	sysuser "github.com/manchtools/cadestro/sdk/sys/user"
)

func ws17aULID() string { return ulid.Make().String() }

func TestTerminal_Start_RejectsPrefixedButInvalidUsername(t *testing.T) {
	cases := map[string]string{
		"uppercase":  "cadestro-tty-Abc",
		"slash":      "cadestro-tty-a/b",
		"newline":    "cadestro-tty-a\nb",
		"colon":      "cadestro-tty-a:b",
		"over-32":    "cadestro-tty-" + strings.Repeat("a", 40),
		"whitespace": "cadestro-tty-a b",
	}
	for name, user := range cases {
		t.Run(name, func(t *testing.T) {
			h, sender := newTestHandler(t)
			err := h.OnTerminalStart(context.Background(), &pb.TerminalStart{
				SessionId: &pb.SessionId{Value: ws17aULID()},
				TtyUser:   user,
				Cols:      80,
				Rows:      24,
			})
			if err != nil {
				t.Fatalf("OnTerminalStart returned %v", err)
			}
			last := sender.lastState()
			if last == nil || last.State != pb.TerminalSessionState_TERMINAL_SESSION_STATE_ERROR {
				t.Fatalf("expected STATE_ERROR, got %v", last)
			}
			if !strings.Contains(last.Error, "invalid tty username") {
				t.Errorf("error = %q, want 'invalid tty username'", last.Error)
			}
			if got := len(h.terminals); got != 0 {
				t.Errorf("registry must stay empty, got %d entries", got)
			}
		})
	}
}

func TestTerminal_Start_RejectsNonUlidSessionId(t *testing.T) {
	bad := map[string]string{
		"parent-traversal": "../../etc",
		"embedded-slash":   "a/b",
		"dotted":           "a.b",
		"empty":            "",
		"too-long":         strings.Repeat("Z", 40),
		"embedded-nul":     "01ARZ3NDEKTSV4RRFFQ69G5F\x00",
	}
	for name, sid := range bad {
		t.Run(name, func(t *testing.T) {
			h, sender := newTestHandler(t)
			err := h.OnTerminalStart(context.Background(), &pb.TerminalStart{
				SessionId: &pb.SessionId{Value: sid},
				TtyUser:   "cadestro-tty-test",
				Cols:      80,
				Rows:      24,
			})
			if err != nil {
				t.Fatalf("OnTerminalStart returned %v", err)
			}
			last := sender.lastState()
			if last == nil || last.State != pb.TerminalSessionState_TERMINAL_SESSION_STATE_ERROR {
				t.Fatalf("expected STATE_ERROR for session id %q, got %v", sid, last)
			}
			if !strings.Contains(last.Error, "invalid session id") {
				t.Errorf("error = %q, want 'invalid session id'", last.Error)
			}
			if got := len(h.terminals); got != 0 {
				t.Errorf("registry must stay empty, got %d entries", got)
			}
		})
	}

	t.Run("valid ULID passes the session-id gate", func(t *testing.T) {
		origGet := sysuserGet
		origModify := sysuserModify
		t.Cleanup(func() { sysuserGet = origGet; sysuserModify = origModify })
		sysuserGet = func(context.Context, string) (sysuser.Info, error) { return sysuser.Info{Locked: false}, nil }
		sysuserModify = func(context.Context, string, sysuser.ModifyOptions) error {
			return fmt.Errorf("usermod unavailable in test")
		}
		h, sender := newTestHandler(t)
		err := h.OnTerminalStart(context.Background(), &pb.TerminalStart{
			SessionId: &pb.SessionId{Value: ws17aULID()}, TtyUser: "cadestro-tty-test", Cols: 80, Rows: 24,
		})
		if err != nil {
			t.Fatalf("OnTerminalStart returned %v", err)
		}
		if last := sender.lastState(); last != nil && strings.Contains(last.Error, "invalid session id") {
			t.Errorf("a valid ULID must pass the session-id gate, got %q", last.Error)
		}
	})
}

func TestTerminal_Start_RejectsAtSessionLimit(t *testing.T) {
	origGet := sysuserGet
	t.Cleanup(func() { sysuserGet = origGet })
	sysuserGet = func(context.Context, string) (sysuser.Info, error) { return sysuser.Info{Locked: false}, nil }

	h, sender := newTestHandler(t)
	h.terminalLimit = 2
	addTestSession(h, ws17aULID(), "cadestro-tty-test", time.Now())
	addTestSession(h, ws17aULID(), "cadestro-tty-test", time.Now())

	err := h.OnTerminalStart(context.Background(), &pb.TerminalStart{
		SessionId: &pb.SessionId{Value: ws17aULID()}, TtyUser: "cadestro-tty-test", Cols: 80, Rows: 24,
	})
	if err != nil {
		t.Fatalf("OnTerminalStart returned %v", err)
	}
	last := sender.lastState()
	if last == nil || last.State != pb.TerminalSessionState_TERMINAL_SESSION_STATE_ERROR {
		t.Fatalf("expected STATE_ERROR at the session limit, got %v", last)
	}
	if !strings.Contains(last.Error, "session limit reached") {
		t.Errorf("error = %q, want 'session limit reached'", last.Error)
	}
	if got := len(h.terminals); got != 2 {
		t.Errorf("a rejected over-limit start must not be registered; registry size = %d, want 2", got)
	}
}

func TestTerminal_Start_RejectsDuplicateSession(t *testing.T) {
	origGet := sysuserGet
	t.Cleanup(func() { sysuserGet = origGet })
	sysuserGet = func(context.Context, string) (sysuser.Info, error) { return sysuser.Info{Locked: false}, nil }

	h, sender := newTestHandler(t)
	dup := ws17aULID()
	addTestSession(h, dup, "cadestro-tty-test", time.Now())

	err := h.OnTerminalStart(context.Background(), &pb.TerminalStart{
		SessionId: &pb.SessionId{Value: dup}, TtyUser: "cadestro-tty-test", Cols: 80, Rows: 24,
	})
	if err != nil {
		t.Fatalf("OnTerminalStart returned %v", err)
	}
	last := sender.lastState()
	if last == nil || last.State != pb.TerminalSessionState_TERMINAL_SESSION_STATE_ERROR {
		t.Fatalf("expected STATE_ERROR for a duplicate session, got %v", last)
	}
	if !strings.Contains(last.Error, "session already exists") {
		t.Errorf("error = %q, want 'session already exists'", last.Error)
	}
	if got := len(h.terminals); got != 1 {
		t.Errorf("the duplicate must not add a second entry; registry size = %d, want 1", got)
	}
}

func TestTerminal_Start_RejectsLockedTtyUser(t *testing.T) {
	origGet := sysuserGet
	origModify := sysuserModify
	t.Cleanup(func() { sysuserGet = origGet; sysuserModify = origModify })

	t.Run("locked user is rejected, nothing reserved", func(t *testing.T) {
		sysuserGet = func(context.Context, string) (sysuser.Info, error) { return sysuser.Info{Locked: true}, nil }
		h, sender := newTestHandler(t)
		err := h.OnTerminalStart(context.Background(), &pb.TerminalStart{
			SessionId: &pb.SessionId{Value: ws17aULID()}, TtyUser: "cadestro-tty-test", Cols: 80, Rows: 24,
		})
		if err != nil {
			t.Fatalf("OnTerminalStart returned %v", err)
		}
		last := sender.lastState()
		if last == nil || last.State != pb.TerminalSessionState_TERMINAL_SESSION_STATE_ERROR {
			t.Fatalf("expected STATE_ERROR for a locked user, got %v", last)
		}
		if !strings.Contains(last.Error, "is disabled") {
			t.Errorf("error = %q, want 'is disabled'", last.Error)
		}
		if got := len(h.terminals); got != 0 {
			t.Errorf("a locked-user rejection must reserve no slot, got %d", got)
		}
	})

	t.Run("unlocked user passes the locked gate (fails later at shell activation)", func(t *testing.T) {
		sysuserGet = func(context.Context, string) (sysuser.Info, error) { return sysuser.Info{Locked: false}, nil }
		sysuserModify = func(context.Context, string, sysuser.ModifyOptions) error {
			return fmt.Errorf("usermod boom")
		}
		h, sender := newTestHandler(t)
		err := h.OnTerminalStart(context.Background(), &pb.TerminalStart{
			SessionId: &pb.SessionId{Value: ws17aULID()}, TtyUser: "cadestro-tty-test", Cols: 80, Rows: 24,
		})
		if err != nil {
			t.Fatalf("OnTerminalStart returned %v", err)
		}
		last := sender.lastState()
		if last == nil || last.State != pb.TerminalSessionState_TERMINAL_SESSION_STATE_ERROR {
			t.Fatalf("expected STATE_ERROR at shell activation, got %v", last)
		}
		if strings.Contains(last.Error, "is disabled") {
			t.Errorf("an unlocked user must pass the locked gate, got %q", last.Error)
		}
		if !strings.Contains(last.Error, "activate shell") {
			t.Errorf("error = %q, want it to fail at shell activation (proving the locked gate was passed)", last.Error)
		}
		if got := len(h.terminals); got != 0 {
			t.Errorf("a failed start must unwind its reserved slot, got %d", got)
		}
	})
}
