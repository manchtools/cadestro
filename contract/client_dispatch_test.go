package contract

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

type blockingFanoutHandler struct {
	release  chan struct{}
	inFlight int32
	maxSeen  int32
	entered  int32
}

func (h *blockingFanoutHandler) enter() {
	cur := atomic.AddInt32(&h.inFlight, 1)
	atomic.AddInt32(&h.entered, 1)
	for {
		old := atomic.LoadInt32(&h.maxSeen)
		if cur <= old || atomic.CompareAndSwapInt32(&h.maxSeen, old, cur) {
			break
		}
	}
	<-h.release
	atomic.AddInt32(&h.inFlight, -1)
}

func (h *blockingFanoutHandler) OnWelcome(ctx context.Context, w *cadestrov1.Welcome) error {
	return nil
}
func (h *blockingFanoutHandler) OnQuery(ctx context.Context, q *cadestrov1.OSQuery) (*cadestrov1.OSQueryResult, error) {
	return nil, nil
}
func (h *blockingFanoutHandler) OnError(ctx context.Context, e *cadestrov1.Error) error { return nil }

func (h *blockingFanoutHandler) CollectInventory(ctx context.Context) *cadestrov1.DeviceInventory {
	return nil
}

func (h *blockingFanoutHandler) OnRequestInventory(ctx context.Context, req *cadestrov1.RequestInventory) *cadestrov1.DeviceInventory {
	h.enter()
	return nil
}

func (h *blockingFanoutHandler) OnRevokeLuksDeviceKey(ctx context.Context, req *cadestrov1.RevokeLuksDeviceKey) (bool, string) {
	h.enter()
	return false, "blocked"
}

func (h *blockingFanoutHandler) OnSyncDevice(context.Context, *cadestrov1.SyncDeviceCommand) error {
	h.enter()
	return nil
}

func (h *blockingFanoutHandler) OnRebootDevice(context.Context, *cadestrov1.RebootDeviceCommand) error {
	h.enter()
	return nil
}

func waitForCond(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("condition not met within deadline")
}

func TestDispatchServerMessage_InventoryConcurrencyBounded(t *testing.T) {
	c := NewClient("https://gw.invalid", WithAuth("01HZZZZZZZZZZZZZZZZZZZZZZZZ", ""))
	h := &blockingFanoutHandler{release: make(chan struct{})}

	const fired = 50
	for i := 0; i < fired; i++ {
		msg := &cadestrov1.ServerMessage{
			Id: &cadestrov1.MessageId{Value: "m"},
			Payload: &cadestrov1.ServerMessage_RequestInventory{
				RequestInventory: &cadestrov1.RequestInventory{QueryId: &cadestrov1.QueryId{Value: "01HQ0000000000000000000000"}},
			},
		}
		if err := c.dispatchServerMessage(context.Background(), msg, h); err != nil {
			t.Fatalf("dispatch: %v", err)
		}
	}

	cap := inventoryDispatchConcurrency
	if cap < 1 {
		t.Fatalf("inventoryDispatchConcurrency = %d, want >= 1", cap)
	}

	waitForCond(t, func() bool { return atomic.LoadInt32(&h.entered) >= int32(cap) })
	time.Sleep(100 * time.Millisecond)

	if got := atomic.LoadInt32(&h.maxSeen); got != int32(cap) {
		t.Errorf("peak concurrent inventory handlers = %d, want %d", got, cap)
	}
	if got := atomic.LoadInt32(&h.entered); got != int32(cap) {
		t.Errorf("total inventory handlers entered = %d, want %d (excess of %d must be dropped)", got, cap, fired-cap)
	}

	close(h.release)
	waitForCond(t, func() bool { return atomic.LoadInt32(&h.inFlight) == 0 })
}

func TestDispatchServerMessage_LiveControlIsSingleFlight(t *testing.T) {
	c := NewClient("https://control.invalid", WithAuth("01HZZZZZZZZZZZZZZZZZZZZZZZZ", ""))
	h := &blockingFanoutHandler{release: make(chan struct{})}
	first := &cadestrov1.ServerMessage{Id: &cadestrov1.MessageId{Value: NewULID()}, Payload: &cadestrov1.ServerMessage_SyncDevice{SyncDevice: &cadestrov1.SyncDeviceCommand{}}}
	if err := c.dispatchServerMessage(context.Background(), first, h); err != nil {
		t.Fatal(err)
	}
	waitForCond(t, func() bool { return atomic.LoadInt32(&h.entered) == 1 })

	second := &cadestrov1.ServerMessage{Id: &cadestrov1.MessageId{Value: NewULID()}, Payload: &cadestrov1.ServerMessage_RebootDevice{RebootDevice: &cadestrov1.RebootDeviceCommand{}}}
	if err := c.dispatchServerMessage(context.Background(), second, h); err == nil {
		t.Fatal("busy live control must send a correlated failure")
	}
	if got := atomic.LoadInt32(&h.entered); got != 1 {
		t.Fatalf("busy live control ran another handler: entered=%d", got)
	}
	close(h.release)
	waitForCond(t, func() bool { return atomic.LoadInt32(&h.inFlight) == 0 })
}

func TestDispatchServerMessage_LuksRevokeConcurrencyBounded(t *testing.T) {
	c := NewClient("https://gw.invalid", WithAuth("01HZZZZZZZZZZZZZZZZZZZZZZZZ", ""))
	h := &blockingFanoutHandler{release: make(chan struct{})}

	const fired = 50
	for i := 0; i < fired; i++ {
		msg := &cadestrov1.ServerMessage{
			Id: &cadestrov1.MessageId{Value: "m"},
			Payload: &cadestrov1.ServerMessage_RevokeLuksDeviceKey{
				RevokeLuksDeviceKey: &cadestrov1.RevokeLuksDeviceKey{ActionId: &cadestrov1.ActionId{Value: "01HQ0000000000000000000000"}},
			},
		}
		if err := c.dispatchServerMessage(context.Background(), msg, h); err != nil {
			t.Fatalf("dispatch: %v", err)
		}
	}

	cap := luksRevokeDispatchConcurrency
	if cap < 1 {
		t.Fatalf("luksRevokeDispatchConcurrency = %d, want >= 1", cap)
	}
	waitForCond(t, func() bool { return atomic.LoadInt32(&h.entered) >= int32(cap) })
	time.Sleep(100 * time.Millisecond)

	if got := atomic.LoadInt32(&h.maxSeen); got != int32(cap) {
		t.Errorf("peak concurrent luks-revoke handlers = %d, want %d", got, cap)
	}
	if got := atomic.LoadInt32(&h.entered); got != int32(cap) {
		t.Errorf("total luks-revoke handlers entered = %d, want %d (excess dropped)", got, cap)
	}

	close(h.release)
	waitForCond(t, func() bool { return atomic.LoadInt32(&h.inFlight) == 0 })
}
