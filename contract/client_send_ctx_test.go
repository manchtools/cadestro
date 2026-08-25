package contract

import (
	"context"
	"errors"
	"testing"
	"time"

	"connectrpc.com/connect"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

func newStalledLoopback(t *testing.T) *agentLoopback {
	t.Helper()
	l := newAgentLoopback(t)
	l.handler.onStream = func(ctx context.Context, _ *connect.BidiStream[cadestrov1.AgentMessage, cadestrov1.ServerMessage]) error {

		<-ctx.Done()
		return nil
	}
	return l
}

func connectCancellable(t *testing.T, c *Client) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	if err := c.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() {
		cancel()
		done := make(chan struct{})
		go func() { _ = c.Close(); close(done) }()
		select {
		case <-done:
		case <-time.After(3 * time.Second):

		}
	})
}

func bigTerminalOutput() *cadestrov1.TerminalOutput {
	return &cadestrov1.TerminalOutput{
		SessionId: &cadestrov1.SessionId{Value: "01ARZ3NDEKTSV4RRFFQ69G5FAV"},
		Data:      make([]byte, 2<<20),
	}
}

func TestSendTerminalOutput_HonorsContextDeadline_WhenPeerNotDraining(t *testing.T) {
	l := newStalledLoopback(t)
	c := l.newClient(WithAuth("device-x", "tok"))

	connectCancellable(t, c)

	t.Run("already cancelled returns Canceled before sending", func(t *testing.T) {
		cctx, cancel := context.WithCancel(context.Background())
		cancel()
		done := make(chan error, 1)
		go func() { done <- c.SendTerminalOutput(cctx, bigTerminalOutput()) }()
		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("want context.Canceled, got %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("already-cancelled SendTerminalOutput blocked instead of refusing — finding #1 (ctx ignored)")
		}
	})

	t.Run("deadline surfaces as DeadlineExceeded", func(t *testing.T) {
		done := make(chan error, 1)
		start := time.Now()
		go func() {
			sctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
			defer cancel()
			done <- c.SendTerminalOutput(sctx, bigTerminalOutput())
		}()

		select {
		case err := <-done:
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("want context.DeadlineExceeded, got %v", err)
			}
			if elapsed := time.Since(start); elapsed > 2*time.Second {
				t.Errorf("send took %v, expected to abort near the 200ms deadline", elapsed)
			}
		case <-time.After(4 * time.Second):
			t.Fatal("SendTerminalOutput blocked far past its ctx deadline — finding #1 (ctx ignored / sendMu wedged)")
		}
	})
}

func TestSend_DoesNotSerializeAllTrafficBehindOneStalledSend(t *testing.T) {
	l := newStalledLoopback(t)
	c := l.newClient(WithAuth("device-x", "tok"))

	connectCancellable(t, c)

	blockerDone := make(chan struct{})
	go func() {
		defer close(blockerDone)

		_ = c.SendTerminalOutput(context.Background(), bigTerminalOutput())
	}()

	time.Sleep(150 * time.Millisecond)

	done := make(chan error, 1)
	start := time.Now()
	go func() {
		vctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
		defer cancel()
		done <- c.SendHeartbeat(vctx, &cadestrov1.Heartbeat{})
	}()

	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("want context.DeadlineExceeded for the queued victim, got %v", err)
		}
		if elapsed := time.Since(start); elapsed > 2*time.Second {
			t.Errorf("victim send took %v, expected to abort near its 150ms deadline", elapsed)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("queued SendHeartbeat inherited the blocker's stall — finding #1 (one stalled send serializes all traffic)")
	}
}

func TestSendTerminalStateChange_HonorsContextDeadline(t *testing.T) {
	l := newStalledLoopback(t)
	c := l.newClient(WithAuth("device-x", "tok"))

	connectCancellable(t, c)

	go func() { _ = c.SendTerminalOutput(context.Background(), bigTerminalOutput()) }()
	time.Sleep(150 * time.Millisecond)

	done := make(chan error, 1)
	go func() {
		sctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
		defer cancel()
		done <- c.SendTerminalStateChange(sctx, &cadestrov1.TerminalStateChange{
			SessionId: &cadestrov1.SessionId{Value: "01ARZ3NDEKTSV4RRFFQ69G5FAV"},
			State:     cadestrov1.TerminalSessionState_TERMINAL_SESSION_STATE_EXITED,
		})
	}()

	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("want context.DeadlineExceeded, got %v", err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("SendTerminalStateChange ignored its ctx deadline — finding #1 (F053 fix is cosmetic)")
	}
}

func TestSend_DrainingPeer_NoRegression(t *testing.T) {
	l := newAgentLoopback(t)
	c := l.newClient(WithAuth("device-x", "tok"))

	connectCancellable(t, c)

	if err := c.SendHeartbeat(context.Background(), &cadestrov1.Heartbeat{}); err != nil {
		t.Fatalf("SendHeartbeat against a draining peer should succeed, got %v", err)
	}
}
