package handler

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/manchtools/cadestro/agent/internal/store"
	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

type fakeSender struct {
	mu      sync.Mutex
	outputs []*pb.TerminalOutput
	states  []*pb.TerminalStateChange
}

func (f *fakeSender) SendTerminalOutput(ctx context.Context, out *pb.TerminalOutput) error {
	f.mu.Lock()
	f.outputs = append(f.outputs, out)
	f.mu.Unlock()
	return nil
}

func (f *fakeSender) SendTerminalStateChange(ctx context.Context, change *pb.TerminalStateChange) error {
	f.mu.Lock()
	f.states = append(f.states, change)
	f.mu.Unlock()
	return nil
}

func (f *fakeSender) lastState() *pb.TerminalStateChange {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.states) == 0 {
		return nil
	}
	return f.states[len(f.states)-1]
}

func newTestHandler(t *testing.T) (*Handler, *fakeSender) {
	t.Helper()

	return newTestHandlerWithTTY(t, true)
}

func newTestHandlerWithTTY(t *testing.T, ttyEnabled bool) (*Handler, *fakeSender) {
	t.Helper()
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.SetTTYEnabled(context.Background(), ttyEnabled); err != nil {
		t.Fatalf("set tty toggle: %v", err)
	}
	h := &Handler{
		logger:      slog.Default(),
		connectedCh: make(chan struct{}),
		store:       st,
		now:         time.Now,
	}
	sender := &fakeSender{}
	h.SetTerminalSender(sender)

	t.Cleanup(h.StopTerminalSweeper)
	return h, sender
}

func addTestSession(h *Handler, id, ttyUser string, lastActivity time.Time) *terminalSession {
	ts := &terminalSession{
		id:           id,
		ttyUser:      ttyUser,
		state:        sessionStateActive,
		lastActivity: lastActivity,
		now:          time.Now,
	}
	h.mu.Lock()
	h.terminals[id] = ts
	h.mu.Unlock()
	return ts
}

func TestTerminal_LookupUnknown(t *testing.T) {
	h, _ := newTestHandler(t)
	if got := h.lookupTerminal("nope"); got != nil {
		t.Errorf("lookupTerminal(unknown) = %v, want nil", got)
	}
}

func TestTerminal_OnInput_UnknownIsNoOp(t *testing.T) {
	h, _ := newTestHandler(t)
	err := h.OnTerminalInput(context.Background(), &pb.TerminalInput{
		SessionId: &pb.SessionId{Value: "01ABC"},
		Data:      []byte("hello"),
	})
	if err != nil {
		t.Errorf("OnTerminalInput(unknown) = %v, want nil", err)
	}
}

func TestTerminal_OnResize_UnknownIsNoOp(t *testing.T) {
	h, _ := newTestHandler(t)
	err := h.OnTerminalResize(context.Background(), &pb.TerminalResize{
		SessionId: &pb.SessionId{Value: "01ABC"},
		Cols:      120,
		Rows:      40,
	})
	if err != nil {
		t.Errorf("OnTerminalResize(unknown) = %v, want nil", err)
	}
}

func TestTerminal_OnStop_UnknownIsNoOp(t *testing.T) {
	h, _ := newTestHandler(t)
	err := h.OnTerminalStop(context.Background(), &pb.TerminalStop{SessionId: &pb.SessionId{Value: "01ABC"}})
	if err != nil {
		t.Errorf("OnTerminalStop(unknown) = %v, want nil", err)
	}
}

func TestTerminal_CloseRemovesFromRegistry(t *testing.T) {
	h, _ := newTestHandler(t)
	addTestSession(h, "01ABC", "cadestro-tty-test", time.Now())

	if got := len(h.terminals); got != 1 {
		t.Fatalf("len(terminals) before close = %d, want 1", got)
	}

	h.closeTerminal(context.Background(), "01ABC", "")

	if got := len(h.terminals); got != 0 {
		t.Errorf("len(terminals) after close = %d, want 0", got)
	}
	if got := h.lookupTerminal("01ABC"); got != nil {
		t.Error("lookup after close should return nil")
	}
}

func TestTerminal_CloseIsIdempotent(t *testing.T) {
	h, _ := newTestHandler(t)
	addTestSession(h, "01ABC", "cadestro-tty-test", time.Now())

	h.closeTerminal(context.Background(), "01ABC", "")

	h.closeTerminal(context.Background(), "01ABC", "")

	if got := len(h.terminals); got != 0 {
		t.Errorf("len(terminals) = %d, want 0", got)
	}
}

func TestTerminal_SweepIdle_ClosesStaleSessions(t *testing.T) {
	h, _ := newTestHandler(t)

	h.mu.Lock()
	h.terminalIdleTimeout = 50 * time.Millisecond
	h.mu.Unlock()

	stale := time.Now().Add(-1 * time.Hour)
	fresh := time.Now()
	addTestSession(h, "stale", "cadestro-tty-a", stale)
	addTestSession(h, "fresh", "cadestro-tty-b", fresh)

	h.sweepIdleTerminals()

	if h.lookupTerminal("stale") != nil {
		t.Error("stale session should have been swept")
	}
	if h.lookupTerminal("fresh") == nil {
		t.Error("fresh session should still be present")
	}
}

func TestTerminal_SweepIdle_LeavesEverythingWhenNothingIsStale(t *testing.T) {
	h, _ := newTestHandler(t)
	h.mu.Lock()
	h.terminalIdleTimeout = 1 * time.Hour
	h.mu.Unlock()

	addTestSession(h, "a", "cadestro-tty-a", time.Now())
	addTestSession(h, "b", "cadestro-tty-b", time.Now())

	h.sweepIdleTerminals()

	if got := len(h.terminals); got != 2 {
		t.Errorf("len(terminals) after no-op sweep = %d, want 2", got)
	}
}

func TestTerminal_SetTerminalSender_AppliesDefaults(t *testing.T) {
	h := &Handler{
		logger:      slog.Default(),
		connectedCh: make(chan struct{}),
		now:         time.Now,
	}
	h.SetTerminalSender(&fakeSender{})

	if h.terminalLimit != defaultTerminalLimit {
		t.Errorf("terminalLimit = %d, want %d", h.terminalLimit, defaultTerminalLimit)
	}
	if h.terminalIdleTimeout != defaultTerminalIdleTimeout {
		t.Errorf("terminalIdleTimeout = %v, want %v", h.terminalIdleTimeout, defaultTerminalIdleTimeout)
	}
	if h.terminals == nil {
		t.Error("terminals map should be initialized")
	}
	if !h.terminalSweeperStarted {
		t.Error("sweeper should have been started")
	}
}

func TestTerminal_SetTerminalSender_DoesNotResetExistingValues(t *testing.T) {
	h := &Handler{
		logger:              slog.Default(),
		connectedCh:         make(chan struct{}),
		terminalLimit:       7,
		terminalIdleTimeout: 5 * time.Minute,
		now:                 time.Now,
	}
	h.SetTerminalSender(&fakeSender{})

	if h.terminalLimit != 7 {
		t.Errorf("terminalLimit was reset to %d", h.terminalLimit)
	}
	if h.terminalIdleTimeout != 5*time.Minute {
		t.Errorf("terminalIdleTimeout was reset to %v", h.terminalIdleTimeout)
	}
}

func TestTerminal_FailStart_EmitsErrorState(t *testing.T) {
	h, sender := newTestHandler(t)
	h.failTerminalStart(context.Background(), sender, "01ABC", "test failure")

	last := sender.lastState()
	if last == nil {
		t.Fatal("expected a state change to be sent")
	}
	if last.GetSessionId().GetValue() != "01ABC" {
		t.Errorf("session_id = %q, want 01ABC", last.SessionId)
	}
	if last.State != pb.TerminalSessionState_TERMINAL_SESSION_STATE_ERROR {
		t.Errorf("state = %v, want ERROR", last.State)
	}
	if last.Error != "test failure" {
		t.Errorf("error = %q, want %q", last.Error, "test failure")
	}
}

func TestTerminal_Start_RejectsNonPrefixedUsername(t *testing.T) {
	h, sender := newTestHandler(t)
	err := h.OnTerminalStart(context.Background(), &pb.TerminalStart{
		SessionId: &pb.SessionId{Value: "01ABC"},
		TtyUser:   "alice",
		Cols:      80,
		Rows:      24,
	})
	if err != nil {
		t.Fatalf("OnTerminalStart returned %v", err)
	}
	last := sender.lastState()
	if last == nil {
		t.Fatal("expected STATE_ERROR for non-prefixed username")
	}
	if last.State != pb.TerminalSessionState_TERMINAL_SESSION_STATE_ERROR {
		t.Errorf("state = %v, want ERROR", last.State)
	}
	if !strings.Contains(last.Error, "invalid tty username") {
		t.Errorf("error = %q, want substring 'invalid tty username'", last.Error)
	}

	if got := len(h.terminals); got != 0 {
		t.Errorf("registry should be empty, got %d entries", got)
	}
}

func TestTerminal_Start_RejectsWhenTTYDisabled(t *testing.T) {
	h, sender := newTestHandlerWithTTY(t, false)
	err := h.OnTerminalStart(context.Background(), &pb.TerminalStart{
		SessionId: &pb.SessionId{Value: "01ABC"},
		TtyUser:   "cadestro-tty-test",
		Cols:      80,
		Rows:      24,
	})
	if err != nil {
		t.Fatalf("OnTerminalStart returned %v", err)
	}
	last := sender.lastState()
	if last == nil {
		t.Fatal("expected STATE_ERROR when TTY is disabled")
	}
	if last.State != pb.TerminalSessionState_TERMINAL_SESSION_STATE_ERROR {
		t.Errorf("state = %v, want ERROR", last.State)
	}
	if !strings.Contains(last.Error, "disabled on this device") {
		t.Errorf("error = %q, want opaque disabled message", last.Error)
	}
	if got := len(h.terminals); got != 0 {
		t.Errorf("registry should be empty, got %d entries", got)
	}
}

func TestTerminal_Start_RejectsWhenStoreMissing(t *testing.T) {
	h := &Handler{
		logger:      slog.Default(),
		connectedCh: make(chan struct{}),
		now:         time.Now,
	}
	sender := &fakeSender{}
	h.SetTerminalSender(sender)

	err := h.OnTerminalStart(context.Background(), &pb.TerminalStart{
		SessionId: &pb.SessionId{Value: "01ABC"},
		TtyUser:   "cadestro-tty-test",
		Cols:      80,
		Rows:      24,
	})
	if err != nil {
		t.Fatalf("OnTerminalStart returned %v", err)
	}
	last := sender.lastState()
	if last == nil {
		t.Fatal("expected STATE_ERROR when store is missing")
	}
	if last.State != pb.TerminalSessionState_TERMINAL_SESSION_STATE_ERROR {
		t.Errorf("state = %v, want ERROR", last.State)
	}
	if !strings.Contains(last.Error, "disabled on this device") {
		t.Errorf("error = %q, want opaque disabled message", last.Error)
	}
}

func TestTerminal_CloseDuringStart_MarksStoppingButLeavesRegistryEntry(t *testing.T) {
	h, _ := newTestHandler(t)

	cancelCalled := make(chan struct{}, 1)
	_, cancel := context.WithCancel(context.Background())
	wrappedCancel := func() {
		cancel()
		select {
		case cancelCalled <- struct{}{}:
		default:
		}
	}
	ts := &terminalSession{
		id:      "01ABC",
		ttyUser: "cadestro-tty-test",
		state:   sessionStateStarting,
		cancel:  wrappedCancel,
		now:     time.Now,
	}
	h.mu.Lock()
	h.terminals["01ABC"] = ts
	h.mu.Unlock()

	h.closeTerminal(context.Background(), "01ABC", "user stopped")

	if !ts.isStopping() {
		t.Error("expected session state = stopping after close-during-start")
	}

	select {
	case <-cancelCalled:
	default:
		t.Error("expected ts.cancel to have been called")
	}

	if h.lookupTerminal("01ABC") == nil {
		t.Error("registry entry must still be present until Start cleans up")
	}
}

func TestTerminal_CloseDuringActive_RemovesFromRegistry(t *testing.T) {
	h, _ := newTestHandler(t)
	addTestSession(h, "01ABC", "cadestro-tty-test", time.Now())

	h.closeTerminal(context.Background(), "01ABC", "")

	if h.lookupTerminal("01ABC") != nil {
		t.Error("active session should have been removed from registry")
	}
}

func TestTerminal_SnapshotTerminalSender_ReturnsLatest(t *testing.T) {
	h := &Handler{
		logger:      slog.Default(),
		connectedCh: make(chan struct{}),
		now:         time.Now,
	}
	if got := h.snapshotTerminalSender(); got != nil {
		t.Errorf("snapshot before SetTerminalSender = %v, want nil", got)
	}

	first := &fakeSender{}
	h.SetTerminalSender(first)
	if got := h.snapshotTerminalSender(); got != first {
		t.Errorf("snapshot = %v, want first sender", got)
	}

	second := &fakeSender{}
	h.SetTerminalSender(second)
	if got := h.snapshotTerminalSender(); got != second {
		t.Errorf("snapshot = %v, want second sender (latest wins)", got)
	}
}

func TestTerminal_AnySessionForUserExcept(t *testing.T) {
	h, _ := newTestHandler(t)
	addTestSession(h, "a", "cadestro-tty-alice", time.Now())
	addTestSession(h, "b", "cadestro-tty-alice", time.Now())
	addTestSession(h, "c", "cadestro-tty-bob", time.Now())

	if !h.anySessionForUserExcept("cadestro-tty-alice", "a") {
		t.Error("session b for alice should be visible when excluding a")
	}
	if !h.anySessionForUserExcept("cadestro-tty-alice", "b") {
		t.Error("session a for alice should be visible when excluding b")
	}
	if h.anySessionForUserExcept("cadestro-tty-bob", "c") {
		t.Error("excluding the only bob session should return false")
	}
	if h.anySessionForUserExcept("cadestro-tty-eve", "any") {
		t.Error("user with no sessions should return false")
	}
}
