package handler

import (
	"context"
	"errors"
	"log/slog"
	"os/user"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/sdk/sys/terminal"
	sysuser "github.com/manchtools/cadestro/sdk/sys/user"
)

const setupTestULID = "01ARZ3NDEKTSV4RRFFQ69G5FAV"

func TestOnTerminalStart_BoundedSetupContext(t *testing.T) {
	h, sender := newTestHandlerWithTTY(t, true)

	origGet, origModify, origTimeout := sysuserGet, sysuserModify, terminalSetupTimeout
	t.Cleanup(func() {
		sysuserGet, sysuserModify, terminalSetupTimeout = origGet, origModify, origTimeout
	})

	sysuserGet = func(context.Context, string) (sysuser.Info, error) { return sysuser.Info{Locked: false}, nil }

	modifyEntered := make(chan struct{})
	var modifyOnce sync.Once
	sysuserModify = func(ctx context.Context, _ string, _ sysuser.ModifyOptions) error {
		modifyOnce.Do(func() { close(modifyEntered) })
		<-ctx.Done()
		return ctx.Err()
	}
	terminalSetupTimeout = 100 * time.Millisecond

	done := make(chan error, 1)
	go func() {
		done <- h.OnTerminalStart(context.Background(), &pb.TerminalStart{
			SessionId: &pb.SessionId{Value: setupTestULID}, TtyUser: "cadestro-tty-test", Cols: 80, Rows: 24,
		})
	}()

	select {
	case <-modifyEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("setup never reached the Modify step")
	}

	select {
	case err := <-done:
		require.NoError(t, err, "OnTerminalStart returns nil; failures surface via STATE_ERROR, not a returned error")
	case <-time.After(2 * time.Second):
		t.Fatal("OnTerminalStart did not return after the setup deadline — the dispatch loop would be wedged")
	}

	last := sender.lastState()
	require.NotNil(t, last, "a setup timeout must emit a TerminalStateChange")
	assert.Equal(t, pb.TerminalSessionState_TERMINAL_SESSION_STATE_ERROR, last.State,
		"a setup timeout must surface STATE_ERROR")

	h.mu.Lock()
	_, exists := h.terminals[setupTestULID]
	h.mu.Unlock()
	assert.False(t, exists, "the half-built session must be removed after a setup failure")
}

func TestTerminalCleanupContextSurvivesRequestCancellationButStaysBounded(t *testing.T) {
	originalTimeout := terminalCleanupTimeout
	terminalCleanupTimeout = 50 * time.Millisecond
	t.Cleanup(func() { terminalCleanupTimeout = originalTimeout })

	requestCtx, cancelRequest := context.WithCancel(context.Background())
	cancelRequest()

	cleanupCtx, cancelCleanup := terminalCleanupContext(requestCtx)
	defer cancelCleanup()
	require.NoError(t, cleanupCtx.Err(), "cleanup must survive the failed request context")
	deadline, ok := cleanupCtx.Deadline()
	require.True(t, ok, "cleanup must always carry a deadline")
	require.LessOrEqual(t, time.Until(deadline), terminalCleanupTimeout)

	select {
	case <-cleanupCtx.Done():
		require.ErrorIs(t, cleanupCtx.Err(), context.DeadlineExceeded)
	case <-time.After(time.Second):
		t.Fatal("terminal cleanup context was not bounded")
	}
}

func TestSweepIdleTerminals_BoundsCleanupContext(t *testing.T) {
	originalModify := sysuserModify
	originalTimeout := terminalCleanupTimeout
	t.Cleanup(func() {
		sysuserModify = originalModify
		terminalCleanupTimeout = originalTimeout
	})

	terminalCleanupTimeout = 50 * time.Millisecond
	var cleanupCtx context.Context
	sysuserModify = func(ctx context.Context, _ string, _ sysuser.ModifyOptions) error {
		cleanupCtx = ctx
		return nil
	}

	h, _ := newTestHandler(t)
	h.terminalIdleTimeout = time.Millisecond
	addTestSession(h, "01ARZ3NDEKTSV4RRFFQ69G5FAV", "cadestro-tty-test", time.Now().Add(-time.Hour))
	h.sweepIdleTerminals()

	require.NotNil(t, cleanupCtx)
	deadline, ok := cleanupCtx.Deadline()
	require.True(t, ok)
	require.LessOrEqual(t, time.Until(deadline), terminalCleanupTimeout)
	require.ErrorIs(t, cleanupCtx.Err(), context.Canceled)
}

type terminalContextSender struct {
	outputCtx chan context.Context
	stateCtx  chan context.Context
	release   chan struct{}
}

func (s *terminalContextSender) SendTerminalOutput(ctx context.Context, _ *pb.TerminalOutput) error {
	s.outputCtx <- ctx
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.release:
		return errors.New("released")
	}
}

func (s *terminalContextSender) SendTerminalStateChange(ctx context.Context, _ *pb.TerminalStateChange) error {
	s.stateCtx <- ctx
	return nil
}

func TestPumpTerminalOutput_UsesSessionAndCleanupContexts(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("PTY session test requires Linux")
	}
	current, err := user.Current()
	if err != nil {
		t.Skipf("cannot determine current user: %v", err)
	}
	tm, err := terminal.New()
	if err != nil {
		t.Skipf("cannot build terminal manager: %v", err)
	}
	sess, err := tm.Open(context.Background(), terminal.SessionConfig{User: current.Username})
	if err != nil {
		t.Skipf("cannot start a local PTY session: %v", err)
	}

	originalModify := sysuserModify
	originalTimeout := terminalCleanupTimeout
	t.Cleanup(func() {
		sysuserModify = originalModify
		terminalCleanupTimeout = originalTimeout
		_ = sess.Close()
	})
	terminalCleanupTimeout = 50 * time.Millisecond
	sysuserModify = func(context.Context, string, sysuser.ModifyOptions) error { return nil }

	sender := &terminalContextSender{
		outputCtx: make(chan context.Context, 1),
		stateCtx:  make(chan context.Context, 1),
		release:   make(chan struct{}),
	}
	sessionCtx, cancelSession := context.WithCancel(context.Background())
	ts := &terminalSession{
		id:      "01ARZ3NDEKTSV4RRFFQ69G5FAV",
		ttyUser: current.Username,
		sender:  sender,
		state:   sessionStateActive,
		session: sess,
		cancel:  func() {},
		now:     time.Now,
	}
	h := &Handler{
		logger:    slog.Default(),
		terminals: map[string]*terminalSession{ts.id: ts},
	}

	done := make(chan struct{})
	go func() {
		h.pumpTerminalOutput(sessionCtx, ts)
		close(done)
	}()
	if _, err := sess.Write([]byte("output\n")); err != nil {
		t.Fatal(err)
	}

	var outputCtx context.Context
	select {
	case outputCtx = <-sender.outputCtx:
	case <-time.After(time.Second):
		t.Fatal("terminal output was not sent")
	}
	cancelSession()
	if err := sess.Close(); err != nil {
		t.Fatal(err)
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		close(sender.release)
		<-done
		t.Fatal("terminal output ignored session cancellation")
	}
	if outputCtx == context.Background() {
		t.Fatal("terminal output used a background context")
	}

	stateCtx := <-sender.stateCtx
	deadline, ok := stateCtx.Deadline()
	require.True(t, ok)
	require.LessOrEqual(t, time.Until(deadline), terminalCleanupTimeout)
}
