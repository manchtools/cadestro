package handler

import (
	"context"
	"testing"

	"github.com/manchtools/cadestro/agent/internal/executor"
	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/stretchr/testify/require"
)

func TestSyncDeviceTriggersExistingSyncChannel(t *testing.T) {
	trigger := make(chan struct{}, 1)
	h := &Handler{syncTrigger: trigger}
	require.NoError(t, h.OnSyncDevice(context.Background(), &pb.SyncDeviceCommand{}))
	select {
	case <-trigger:
	default:
		t.Fatal("sync command did not reach the existing sync trigger")
	}
}

func TestRebootDeviceFailsClosedWithoutRunner(t *testing.T) {
	h := &Handler{executor: executor.NewExecutor(nil)}
	require.Error(t, h.OnRebootDevice(context.Background(), &pb.RebootDeviceCommand{}))
}
