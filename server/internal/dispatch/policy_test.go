package dispatch

import (
	"testing"

	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/require"

	pmp "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

func TestStablePolicyIdentityIsReplaySafeAndContentSensitive(t *testing.T) {
	manifest := &pmp.Manifest{
		Provenance: &pmp.ManifestProvenance{ActionId: ulid.Make().String()},
		Schedule:   &pmp.ActionSchedule{IntervalHours: 8},
		Occurrences: []*pmp.ManifestOccurrence{{
			Action:       &pmp.Action{Id: &pmp.ActionId{Value: ulid.Make().String()}, Type: pmp.ActionType_ACTION_TYPE_SYNC},
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
