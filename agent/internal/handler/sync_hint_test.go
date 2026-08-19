package handler

import (
	"context"
	"testing"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/stretchr/testify/require"
)

func TestSyncHintTriggersExistingSyncChannel(t *testing.T) {
	trigger := make(chan struct{}, 1)
	h := &Handler{syncTrigger: trigger}
	require.NoError(t, h.OnSyncHint(context.Background(), &pb.SyncHint{}))
	select {
	case <-trigger:
	default:
		t.Fatal("sync hint did not reach the existing sync trigger")
	}
}
