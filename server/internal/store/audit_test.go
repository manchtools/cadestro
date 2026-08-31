package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	generated "github.com/manchtools/cadestro/server/internal/store/generated"
	"github.com/stretchr/testify/require"
)

func TestAuditEventEnumChecks(t *testing.T) {
	ctx := context.Background()
	database, err := New(ctx, filepath.Join(t.TempDir(), "control.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	base := generated.CreateAuditEventParams{ID: "01K00000000000000000000001", EventType: cadestrov1.AuditEventType_AUDIT_EVENT_TYPE_ACTION_CREATED, StreamType: cadestrov1.AuditStreamType_AUDIT_STREAM_TYPE_ACTION, StreamID: "action", ActorType: cadestrov1.AuditActorType_AUDIT_ACTOR_TYPE_USER, ActorID: "user", OccurredAt: time.Now()}
	invalidEvent := base
	invalidEvent.ID = "01K00000000000000000000002"
	invalidEvent.EventType = cadestrov1.AuditEventType(0)
	require.Error(t, database.Queries().CreateAuditEvent(ctx, invalidEvent))
	invalidStream := base
	invalidStream.ID = "01K00000000000000000000003"
	invalidStream.StreamType = cadestrov1.AuditStreamType(0)
	require.Error(t, database.Queries().CreateAuditEvent(ctx, invalidStream))
	invalidActor := base
	invalidActor.ID = "01K00000000000000000000004"
	invalidActor.ActorType = cadestrov1.AuditActorType(0)
	require.Error(t, database.Queries().CreateAuditEvent(ctx, invalidActor))
	for _, invalid := range []generated.CreateAuditEventParams{
		{ID: "01K00000000000000000000002", EventType: cadestrov1.AuditEventType(20), StreamType: base.StreamType, StreamID: "action", ActorType: base.ActorType, ActorID: "user", OccurredAt: base.OccurredAt},
		{ID: "01K00000000000000000000003", EventType: base.EventType, StreamType: cadestrov1.AuditStreamType(8), StreamID: "action", ActorType: base.ActorType, ActorID: "user", OccurredAt: base.OccurredAt},
		{ID: "01K00000000000000000000004", EventType: base.EventType, StreamType: base.StreamType, StreamID: "action", ActorType: cadestrov1.AuditActorType(4), ActorID: "user", OccurredAt: base.OccurredAt},
	} {
		require.Error(t, database.Queries().CreateAuditEvent(ctx, invalid))
	}
}
