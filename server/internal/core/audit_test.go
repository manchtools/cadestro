package core

import (
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	db "github.com/manchtools/cadestro/server/internal/store/generated"
)

func TestListAuditEventsPreservesTypedEnums(t *testing.T) {
	service, ctx, _, _ := testService(t)
	err := service.store.Queries().CreateAuditEvent(ctx, db.CreateAuditEventParams{
		ID: "01K00000000000000000000001", EventType: cadestrov1.AuditEventType_AUDIT_EVENT_TYPE_ACTION_UPDATED,
		StreamType: cadestrov1.AuditStreamType_AUDIT_STREAM_TYPE_ACTION, StreamID: "action",
		ActorType: cadestrov1.AuditActorType_AUDIT_ACTOR_TYPE_DEVICE, ActorID: "device", OccurredAt: time.Now(),
	})
	require.NoError(t, err)
	response, err := service.ListAuditEvents(ctx, connect.NewRequest(&cadestrov1.ListAuditEventsRequest{}))
	require.NoError(t, err)
	require.Len(t, response.Msg.Events, 1)
	require.Equal(t, cadestrov1.AuditEventType_AUDIT_EVENT_TYPE_ACTION_UPDATED, response.Msg.Events[0].EventType)
	require.Equal(t, cadestrov1.AuditStreamType_AUDIT_STREAM_TYPE_ACTION, response.Msg.Events[0].StreamType)
	require.Equal(t, cadestrov1.AuditActorType_AUDIT_ACTOR_TYPE_DEVICE, response.Msg.Events[0].ActorType)
}
