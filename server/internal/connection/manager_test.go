package connection

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManager_RegisterGet(t *testing.T) {
	m := NewManager()

	agent := m.Register(context.Background(), "device-1", "host1", "1.0.0", nil)
	assert.Equal(t, "device-1", agent.DeviceID)
	assert.Equal(t, "host1", agent.Hostname)
	assert.Equal(t, "1.0.0", agent.Version)
	assert.False(t, agent.ConnectedAt.IsZero())
	assert.False(t, agent.LastSeen.IsZero())

	got, ok := m.Get("device-1")
	require.True(t, ok)
	assert.Equal(t, agent, got)
}

func TestManager_GetNotFound(t *testing.T) {
	m := NewManager()
	_, ok := m.Get("nonexistent")
	assert.False(t, ok)
}

func TestManager_ReplaceExisting(t *testing.T) {
	m := NewManager()

	agent1 := m.Register(context.Background(), "device-1", "host1", "1.0.0", nil)
	agent2 := m.Register(context.Background(), "device-1", "host1", "2.0.0", nil)

	assert.NotEqual(t, agent1, agent2)

	got, ok := m.Get("device-1")
	require.True(t, ok)
	assert.Equal(t, "2.0.0", got.Version)

	select {
	case <-agent1.ctx.Done():

	default:
		t.Error("old agent context should be cancelled")
	}
}

func TestManager_Unregister(t *testing.T) {
	m := NewManager()

	agent := m.Register(context.Background(), "device-1", "host1", "1.0.0", nil)
	m.Unregister("device-1")

	_, ok := m.Get("device-1")
	assert.False(t, ok)

	select {
	case <-agent.ctx.Done():

	default:
		t.Error("agent context should be cancelled after unregister")
	}
}

func TestManager_UnregisterNonexistent(t *testing.T) {
	m := NewManager()
	m.Unregister("nonexistent")
}

func TestManager_Count(t *testing.T) {
	m := NewManager()

	assert.Equal(t, 0, m.Count())

	m.Register(context.Background(), "device-1", "host1", "1.0.0", nil)
	assert.Equal(t, 1, m.Count())

	m.Register(context.Background(), "device-2", "host2", "1.0.0", nil)
	assert.Equal(t, 2, m.Count())

	m.Unregister("device-1")
	assert.Equal(t, 1, m.Count())
}

func TestManager_List(t *testing.T) {
	m := NewManager()

	m.Register(context.Background(), "device-1", "host1", "1.0.0", nil)
	m.Register(context.Background(), "device-2", "host2", "1.0.0", nil)
	m.Register(context.Background(), "device-3", "host3", "1.0.0", nil)

	ids := m.List()
	assert.Len(t, ids, 3)
	assert.Contains(t, ids, "device-1")
	assert.Contains(t, ids, "device-2")
	assert.Contains(t, ids, "device-3")
}

func TestManager_IsConnected(t *testing.T) {
	m := NewManager()

	assert.False(t, m.IsConnected("device-1"))

	m.Register(context.Background(), "device-1", "host1", "1.0.0", nil)
	assert.True(t, m.IsConnected("device-1"))

	m.Unregister("device-1")
	assert.False(t, m.IsConnected("device-1"))
}

func TestManager_UpdateLastSeen(t *testing.T) {
	m := NewManager()

	agent := m.Register(context.Background(), "device-1", "host1", "1.0.0", nil)
	initial := agent.LastSeen

	m.UpdateLastSeen("device-1")

	got, _ := m.Get("device-1")
	assert.True(t, got.LastSeen.After(initial) || got.LastSeen.Equal(initial))
}

func TestManager_UpdateLastSeen_Nonexistent(t *testing.T) {
	m := NewManager()
	m.UpdateLastSeen("nonexistent")
}

func TestManager_LastSeenSnapshotIsAnIndependentCopy(t *testing.T) {
	m := NewManager()
	at := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	m.now = func() time.Time { return at }
	m.Register(context.Background(), "device-1", "host1", "1.0.0", nil)

	snapshot := m.LastSeenSnapshot()
	require.Equal(t, at, snapshot["device-1"])
	delete(snapshot, "device-1")

	_, stillConnected := m.Get("device-1")
	assert.True(t, stillConnected, "mutating a telemetry snapshot must not mutate the manager")
}

func TestManager_SendNotConnected(t *testing.T) {
	m := NewManager()
	err := m.Send("device-1", nil)
	assert.ErrorIs(t, err, ErrAgentNotConnected)
}

func TestManager_Context(t *testing.T) {
	m := NewManager()

	m.Register(context.Background(), "device-1", "host1", "1.0.0", nil)

	ctx, ok := m.Context("device-1")
	require.True(t, ok)
	assert.NotNil(t, ctx)
	assert.NoError(t, ctx.Err())

	m.Unregister("device-1")

	_, ok = m.Context("device-1")
	assert.False(t, ok)
}

func TestManager_ConcurrentAccess(t *testing.T) {
	m := NewManager()
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			m.Register(context.Background(), id, "host", "1.0.0", nil)
		}(string(rune('a' + i)))
	}

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.Count()
			m.List()
			m.IsConnected("a")
		}()
	}

	wg.Wait()
	assert.Equal(t, 50, m.Count())
}

func TestAgent_SendDoesNotBlockForeverOnAStalledDevice(t *testing.T) {
	restore := SendTimeout
	SendTimeout = 50 * time.Millisecond
	t.Cleanup(func() { SendTimeout = restore })

	m := NewManager()
	agent := m.Register(context.Background(), "device-1", "host1", "1.0.0", nil)

	stalledDevice(t, agent)

	done := make(chan error, 1)
	go func() { done <- agent.Send(&cadestrov1.ServerMessage{}) }()

	select {
	case err := <-done:
		assert.ErrorIs(t, err, ErrSendTimeout,
			"a stalled write must be reported as a timeout, not as success")
	case <-time.After(5 * time.Second):
		t.Fatal("Send never returned — a device that stops reading blocks the server indefinitely")
	}

	assert.True(t, agent.Terminated(),
		"a device that cannot accept a frame must be disconnected, not left registered")
}

func TestAgent_StalledDeviceDoesNotBlockAnother(t *testing.T) {
	restore := SendTimeout
	SendTimeout = 50 * time.Millisecond
	t.Cleanup(func() { SendTimeout = restore })

	m := NewManager()
	stalled := m.Register(context.Background(), "device-1", "host1", "1.0.0", nil)
	healthy := m.Register(context.Background(), "device-2", "host2", "1.0.0", nil)

	stalledDevice(t, stalled)

	var delivered atomic.Int32
	healthy.write = func(*cadestrov1.ServerMessage) error { delivered.Add(1); return nil }

	stalledDone := make(chan struct{})
	go func() { defer close(stalledDone); _ = stalled.Send(&cadestrov1.ServerMessage{}) }()
	time.Sleep(10 * time.Millisecond)

	require.NoError(t, healthy.Send(&cadestrov1.ServerMessage{}),
		"a healthy device must be reachable while another is wedged")
	assert.Equal(t, int32(1), delivered.Load())

	<-stalledDone
}

func TestAgent_SendAfterTimeoutIsRefusedNotQueued(t *testing.T) {
	restore := SendTimeout
	SendTimeout = 50 * time.Millisecond
	t.Cleanup(func() { SendTimeout = restore })

	m := NewManager()
	agent := m.Register(context.Background(), "device-1", "host1", "1.0.0", nil)

	writes := stalledDevice(t, agent)

	require.ErrorIs(t, agent.Send(&cadestrov1.ServerMessage{}), ErrSendTimeout)

	start := time.Now()
	err := agent.Send(&cadestrov1.ServerMessage{})
	assert.ErrorIs(t, err, ErrAgentNotConnected)
	assert.Less(t, time.Since(start), SendTimeout,
		"the retry must be refused immediately, not wait out another timeout")
	assert.Equal(t, int32(1), writes.Load(),
		"only the first send may enter the stream — connect streams are not safe for concurrent use")
}

func stalledDevice(t *testing.T, a *Agent) *atomic.Int32 {
	t.Helper()
	var writes atomic.Int32
	deadline := make(chan time.Time, 1)

	a.SetWriteDeadlineFunc(func(d time.Time) error {
		if d.IsZero() {
			return nil
		}
		select {
		case deadline <- d:
		default:
		}
		return nil
	})
	a.write = func(*cadestrov1.ServerMessage) error {
		writes.Add(1)
		select {
		case d := <-deadline:
			time.Sleep(time.Until(d))
			return os.ErrDeadlineExceeded
		case <-time.After(5 * time.Second):
			return errors.New("Send never armed a write deadline — the write would block forever on a real transport")
		}
	}
	return &writes
}

func TestAgent_WaitForInFlightSendBlocksUntilTheWriteReleases(t *testing.T) {
	m := NewManager()
	agent := m.Register(context.Background(), "device-1", "host1", "1.0.0", nil)

	entered := make(chan struct{})
	release := make(chan struct{})
	agent.write = func(*cadestrov1.ServerMessage) error {
		close(entered)
		<-release
		return nil
	}

	sendDone := make(chan struct{})
	go func() { defer close(sendDone); _ = agent.Send(&cadestrov1.ServerMessage{}) }()
	<-entered

	waited := make(chan struct{})
	go func() { defer close(waited); agent.WaitForInFlightSend() }()

	select {
	case <-waited:
		t.Fatal("WaitForInFlightSend returned while the write was still on the wire — " +
			"a handler returning on it would hand the stream back mid-frame")
	case <-time.After(100 * time.Millisecond):
	}

	close(release)
	select {
	case <-waited:
	case <-time.After(5 * time.Second):
		t.Fatal("WaitForInFlightSend never returned after the write finished")
	}
	<-sendDone
}

func TestManager_UnregisterIfCurrentLeavesAReplacementAlone(t *testing.T) {
	m := NewManager()

	old := m.Register(context.Background(), "device-1", "host1", "1.0.0", nil)

	fresh := m.Register(context.Background(), "device-1", "host1", "1.0.1", nil)
	require.NotSame(t, old, fresh)

	assert.False(t, m.UnregisterIfCurrent("device-1", old),
		"the stale handler must report that it removed nothing — it no longer owns the registration")

	got, ok := m.Get("device-1")
	require.True(t, ok, "the reconnected device was unregistered by the handler it replaced")
	assert.Same(t, fresh, got, "the surviving registration must be the new one")
	assert.False(t, fresh.Terminated(), "the replacement's connection was closed by the departing handler")
}

func TestManager_UnregisterIfCurrentRemovesTheLiveRegistration(t *testing.T) {
	m := NewManager()
	agent := m.Register(context.Background(), "device-1", "host1", "1.0.0", nil)

	assert.True(t, m.UnregisterIfCurrent("device-1", agent))
	_, ok := m.Get("device-1")
	assert.False(t, ok, "the live registration must actually be removed")
	assert.True(t, agent.Terminated(), "removing a registration must close its connection")

	assert.False(t, m.UnregisterIfCurrent("device-1", agent),
		"a second teardown must be a no-op, not a removal of whatever is there now")
}
