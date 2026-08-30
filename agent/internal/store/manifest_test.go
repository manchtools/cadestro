package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

func testManifest() *pb.Manifest {
	return &pb.Manifest{
		ManifestId:   &pb.ManifestId{Value: "01K00000000000000000000002"},
		OccurrenceId: &pb.OccurrenceId{Value: "01K00000000000000000000003"},
		Action:       &pb.Action{Id: &pb.ActionId{Value: "01K00000000000000000000004"}, Params: &pb.Action_Update{Update: &pb.UpdateActionParams{}}},
		Schedule:     &pb.ActionSchedule{RunOnAssign: true, IntervalHours: 8},
	}
}

func TestReconcilePolicyReplacesUnassignedWork(t *testing.T) {
	st, err := New(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, st.Close()) })
	manifest := testManifest()
	require.NoError(t, st.ReconcilePolicy(context.Background(), &pb.DesiredPolicy{
		Revision: &pb.PolicyRevisionId{Value: "01K00000000000000000000005"}, Manifests: []*pb.Manifest{manifest},
	}))
	due, err := st.GetDueScheduledWork(context.Background())
	require.NoError(t, err)
	require.Len(t, due, 1)
	require.Equal(t, manifest.GetAction().GetId().GetValue(), due[0].Manifest.GetAction().GetId().GetValue())

	require.NoError(t, st.ReconcilePolicy(context.Background(), &pb.DesiredPolicy{Revision: &pb.PolicyRevisionId{Value: "01K00000000000000000000006"}}))
	due, err = st.GetDueScheduledWork(context.Background())
	require.NoError(t, err)
	require.Empty(t, due)
}

func TestReconcilePolicyRejectsMalformedManifest(t *testing.T) {
	st, err := New(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, st.Close()) })
	err = st.ReconcilePolicy(context.Background(), &pb.DesiredPolicy{
		Revision:  &pb.PolicyRevisionId{Value: "01K00000000000000000000007"},
		Manifests: []*pb.Manifest{{ManifestId: &pb.ManifestId{Value: "01K00000000000000000000008"}}},
	})
	require.ErrorContains(t, err, "malformed manifest")
}

func TestInterruptedActionBecomesIndeterminate(t *testing.T) {
	st, err := New(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, st.Close()) })
	manifest := testManifest()
	require.NoError(t, st.ReconcilePolicy(context.Background(), &pb.DesiredPolicy{
		Revision: &pb.PolicyRevisionId{Value: "01K00000000000000000000009"}, Manifests: []*pb.Manifest{manifest},
	}))
	due, err := st.GetDueScheduledWork(context.Background())
	require.NoError(t, err)
	require.Len(t, due, 1)
	_, err = st.BeginManifestRun(context.Background(), &due[0], time.Now())
	require.NoError(t, err)
	require.NoError(t, st.MarkOccurrenceStarted(context.Background(), due[0].RunID, manifest.GetOccurrenceId().GetValue(), time.Now()))

	recovered, err := st.RecoverInterruptedOccurrences(context.Background())
	require.NoError(t, err)
	require.Len(t, recovered, 1)
	require.Equal(t, pb.ExecutionStatus_EXECUTION_STATUS_INDETERMINATE, recovered[0].ActionResult.GetStatus())

	recovered, err = st.RecoverInterruptedOccurrences(context.Background())
	require.NoError(t, err)
	require.Empty(t, recovered)
}
