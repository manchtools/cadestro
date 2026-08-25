package handler

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
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
