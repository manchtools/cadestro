package dispatch

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/require"

	pmp "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

func TestLiveOperationCorrelationIgnoresLateResultsButRejectsWrongDevice(t *testing.T) {
	h := &Handlers{live: make(map[string]pendingLiveOperation)}
	deviceID, otherDeviceID, operationID := ulid.Make().String(), ulid.Make().String(), ulid.Make().String()
	result := &pmp.SyncDeviceResult{Success: true}

	// A response may race an admin timeout. It is stale, not a reason to tear
	// down the otherwise healthy authenticated agent stream.
	require.NoError(t, h.CompleteSyncDevice(context.Background(), deviceID, operationID, result))

	h.live[operationID] = pendingLiveOperation{
		deviceID: deviceID,
		action:   "SYNC",
		result:   make(chan liveOperationResult, 1),
	}
	err := h.CompleteSyncDevice(context.Background(), otherDeviceID, operationID, result)
	require.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
	require.Contains(t, h.live, operationID)
}

func TestStablePolicyIdentityIsReplaySafeAndContentSensitive(t *testing.T) {
	manifest := &pmp.Manifest{
		Provenance: &pmp.ManifestProvenance{ActionId: ulid.Make().String()},
		Schedule:   &pmp.ActionSchedule{IntervalHours: 8},
		Occurrences: []*pmp.ManifestOccurrence{{
			Action:       &pmp.Action{Id: &pmp.ActionId{Value: ulid.Make().String()}, Type: pmp.ActionType_ACTION_TYPE_UPDATE},
			OccurrenceId: ulid.Make().String(),
		}},
	}
	stablePolicyIdentity(manifest)
	firstID, firstOccurrence := manifest.ManifestId, manifest.Occurrences[0].OccurrenceId
	stablePolicyIdentity(manifest)
	require.Equal(t, firstID, manifest.ManifestId)
	require.Equal(t, firstOccurrence, manifest.Occurrences[0].OccurrenceId)
	if _, err := ulid.ParseStrict(firstID); err != nil {
		t.Fatalf("manifest identity is not a strict ULID: %v", err)
	}

	manifest.Schedule.IntervalHours = 24
	stablePolicyIdentity(manifest)
	require.NotEqual(t, firstID, manifest.ManifestId)
}
