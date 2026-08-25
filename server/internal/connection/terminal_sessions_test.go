package connection

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

func newTestSession(id string) *TerminalSession {
	return NewTerminalSession(id, "dev-1", "user-1", "cadestro-tty-alice", 80, 24)
}

func outputMsg() *cadestrov1.AgentMessage {
	return &cadestrov1.AgentMessage{
		Payload: &cadestrov1.AgentMessage_TerminalOutput{
			TerminalOutput: &cadestrov1.TerminalOutput{SessionId: &cadestrov1.SessionId{Value: "s1"}, Data: []byte("x")},
		},
	}
}

func TestTerminalSessionRegistry_ConcurrentRouteAndUnregister(t *testing.T) {
	for iter := 0; iter < 200; iter++ {
		r := NewTerminalSessionRegistry()
		s := newTestSession("s1")
		r.Register(s)

		drainDone := make(chan struct{})
		go func() {
			for range s.OutputCh {
			}
			close(drainDone)
		}()

		var wg sync.WaitGroup
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < 50; j++ {
					r.RouteAgentMessage("s1", outputMsg())
				}
			}()
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.Unregister("s1")
		}()
		wg.Wait()

		r.Unregister("s1")
		<-drainDone
	}
}

func TestTerminalSessionRegistry_ReregisterSameIDClosesOldChannel(t *testing.T) {
	r := NewTerminalSessionRegistry()
	old := newTestSession("s1")
	r.Register(old)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < 100; j++ {
			r.RouteAgentMessage("s1", outputMsg())
		}
	}()

	fresh := newTestSession("s1")
	r.Register(fresh)
	wg.Wait()

	drained := make(chan struct{})
	go func() {
		for range old.OutputCh {
		}
		close(drained)
	}()
	select {
	case <-drained:
	case <-time.After(time.Second):
		t.Fatal("re-register must close the old session's channel")
	}

	require.True(t, r.RouteAgentMessage("s1", outputMsg()))
	select {
	case msg := <-fresh.OutputCh:
		assert.NotNil(t, msg)
	case <-time.After(time.Second):
		t.Fatal("reader on the new channel did not receive a routed frame")
	}
}

func TestTerminalSessionRegistry_UnregisterIdempotent(t *testing.T) {
	r := NewTerminalSessionRegistry()
	r.Register(newTestSession("s1"))

	assert.NotPanics(t, func() {
		r.Unregister("s1")
		r.Unregister("s1")
		r.Unregister("never-registered")
	})
}

func TestTerminalSessionRegistry_RouteAfterUnregisterReturnsFalse(t *testing.T) {
	r := NewTerminalSessionRegistry()

	assert.False(t, r.RouteAgentMessage("missing", outputMsg()))

	r.Register(newTestSession("s1"))
	assert.True(t, r.RouteAgentMessage("s1", outputMsg()))

	r.Unregister("s1")
	assert.False(t, r.RouteAgentMessage("s1", outputMsg()))
}
