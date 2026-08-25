package handler

import (
	"context"
	"os/user"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/manchtools/cadestro/sdk/sys/terminal"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

const dimsErrFragment = "invalid terminal dimensions"

func TestValidateDims(t *testing.T) {
	cases := []struct {
		name       string
		cols, rows uint32
		wantErr    bool
	}{
		{"correct mid-range", 80, 24, false},
		{"correct min", 1, 1, false},
		{"correct max", 65535, 65535, false},
		{"absent: cols zero", 0, 24, true},
		{"absent: rows zero", 80, 0, true},
		{"absent: both zero", 0, 0, true},
		{"wrong: cols 65536 wraps to 0", 65536, 24, true},
		{"wrong: cols 65537 wraps to 1", 65537, 24, true},
		{"wrong: rows 65536 wraps to 0", 80, 65536, true},
		{"wrong: cols huge", 1 << 20, 24, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateDims(tc.cols, tc.rows)
			if tc.wantErr && err == nil {
				t.Fatalf("validateDims(%d,%d) = nil, want error", tc.cols, tc.rows)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("validateDims(%d,%d) = %v, want nil", tc.cols, tc.rows, err)
			}
		})
	}
}

func TestOnTerminalStart_ColsRowsBounds(t *testing.T) {
	const sessID = "01HSTARTBOUNDS00000000000"

	t.Run("present-but-wrong: 65536 (wraps to 0) is rejected, not a 0xN PTY", func(t *testing.T) {
		h, sender := newTestHandlerWithTTY(t, true)
		err := h.OnTerminalStart(context.Background(), &pb.TerminalStart{
			SessionId: &pb.SessionId{Value: sessID},
			TtyUser:   terminal.TTYUsernamePrefix + "bounds",
			Cols:      65536,
			Rows:      24,
		})
		if err != nil {
			t.Fatalf("OnTerminalStart returned fatal err: %v", err)
		}
		assertDimsRejected(t, sender)
	})

	t.Run("present-but-wrong: 65537 (wraps to 1) is rejected", func(t *testing.T) {
		h, sender := newTestHandlerWithTTY(t, true)
		err := h.OnTerminalStart(context.Background(), &pb.TerminalStart{
			SessionId: &pb.SessionId{Value: sessID},
			TtyUser:   terminal.TTYUsernamePrefix + "bounds",
			Cols:      65537,
			Rows:      24,
		})
		if err != nil {
			t.Fatalf("OnTerminalStart returned fatal err: %v", err)
		}
		assertDimsRejected(t, sender)
	})

	t.Run("absent: zero dims are rejected (no 0x0 PTY)", func(t *testing.T) {
		h, sender := newTestHandlerWithTTY(t, true)
		err := h.OnTerminalStart(context.Background(), &pb.TerminalStart{
			SessionId: &pb.SessionId{Value: sessID},
			TtyUser:   terminal.TTYUsernamePrefix + "bounds",
			Cols:      0,
			Rows:      0,
		})
		if err != nil {
			t.Fatalf("OnTerminalStart returned fatal err: %v", err)
		}
		assertDimsRejected(t, sender)
	})

	t.Run("correct: in-range dims pass the dims gate (fail later for a NON-dims reason)", func(t *testing.T) {
		h, sender := newTestHandlerWithTTY(t, true)
		err := h.OnTerminalStart(context.Background(), &pb.TerminalStart{
			SessionId: &pb.SessionId{Value: sessID},
			TtyUser:   terminal.TTYUsernamePrefix + "definitely-not-provisioned",
			Cols:      80,
			Rows:      24,
		})
		if err != nil {
			t.Fatalf("OnTerminalStart returned fatal err: %v", err)
		}
		last := sender.lastState()
		if last == nil {

			return
		}

		if strings.Contains(strings.ToLower(last.Error), dimsErrFragment) {
			t.Fatalf("in-range 80x24 was rejected as bad dimensions: %q", last.Error)
		}
	})
}

func assertDimsRejected(t *testing.T, sender *fakeSender) {
	t.Helper()
	last := sender.lastState()
	if last == nil {
		t.Fatal("expected a STATE_ERROR for out-of-range dimensions, got none")
	}
	if last.State != pb.TerminalSessionState_TERMINAL_SESSION_STATE_ERROR {
		t.Fatalf("state = %v, want STATE_ERROR", last.State)
	}
	if !strings.Contains(strings.ToLower(last.Error), dimsErrFragment) {
		t.Fatalf("error reason = %q, want it to mention %q", last.Error, dimsErrFragment)
	}
}

func TestOnTerminalResize_ColsRowsBounds(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("PTY session test requires Linux")
	}
	cur, err := user.Current()
	if err != nil {
		t.Skipf("cannot determine current user: %v", err)
	}

	h, _ := newTestHandlerWithTTY(t, true)

	tm, err := terminal.New()
	if err != nil {
		t.Skipf("cannot build terminal manager: %v", err)
	}
	sess, err := tm.Open(context.Background(), terminal.SessionConfig{User: cur.Username})
	if err != nil {
		t.Skipf("cannot start a local PTY session: %v", err)
	}
	defer sess.Close()

	const sessID = "01HRESIZEBOUNDS0000000000"
	ts := addTestSession(h, sessID, cur.Username, time.Now())
	ts.mu.Lock()
	ts.session = sess
	ts.mu.Unlock()

	t.Run("correct: in-range resize is applied", func(t *testing.T) {
		err := h.OnTerminalResize(context.Background(), &pb.TerminalResize{
			SessionId: &pb.SessionId{Value: sessID}, Cols: 120, Rows: 40,
		})
		if err != nil {
			t.Fatalf("in-range resize errored: %v", err)
		}
	})

	t.Run("present-but-wrong: 65536 (wraps to 0) is rejected, Resize not called truncated", func(t *testing.T) {
		err := h.OnTerminalResize(context.Background(), &pb.TerminalResize{
			SessionId: &pb.SessionId{Value: sessID}, Cols: 65536, Rows: 40,
		})
		if err != nil {
			t.Fatalf("out-of-range resize must be a non-fatal no-op, got: %v", err)
		}

		if err := h.OnTerminalResize(context.Background(), &pb.TerminalResize{
			SessionId: &pb.SessionId{Value: sessID}, Cols: 100, Rows: 30,
		}); err != nil {
			t.Fatalf("session unusable after out-of-range resize: %v", err)
		}
	})

	t.Run("absent: zero dims rejected as a no-op", func(t *testing.T) {
		if err := h.OnTerminalResize(context.Background(), &pb.TerminalResize{
			SessionId: &pb.SessionId{Value: sessID}, Cols: 0, Rows: 0,
		}); err != nil {
			t.Fatalf("zero-dims resize must be a non-fatal no-op, got: %v", err)
		}
	})
}
