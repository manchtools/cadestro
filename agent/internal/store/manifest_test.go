package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

func testManifest() *pb.Manifest {
	return &pb.Manifest{
		ManifestId: "01K00000000000000000000002",
		Schedule:   &pb.ActionSchedule{RunOnAssign: true, IntervalHours: 8},
		Occurrences: []*pb.ManifestOccurrence{{
			OccurrenceId: "01K00000000000000000000003",
			Action: &pb.Action{
				Id:   &pb.ActionId{Value: "01K00000000000000000000004"},
				Type: pb.ActionType_ACTION_TYPE_UPDATE,
			},
		}},
	}
}
func TestReconcilePolicyIsReceiptFreeAndRemovesUnassignedWork(t *testing.T) {
	st, err := New(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, st.Close()) })

	manifest := testManifest()
	manifest.ManifestId = "01K00000000000000000000012"
	manifest.Occurrences[0].OccurrenceId = "01K00000000000000000000013"
	policy := &pb.DesiredPolicy{Revision: "01K00000000000000000000014", Manifests: []*pb.Manifest{manifest}}
	require.NoError(t, st.ReconcilePolicy(context.Background(), policy))

	due, err := st.GetDueScheduledWork(context.Background())
	require.NoError(t, err)
	require.Len(t, due, 1)
	require.Equal(t, manifest.ManifestId, due[0].Manifest.GetManifestId())

	// The same Sync snapshot is idempotent and does not create another run.
	require.NoError(t, st.ReconcilePolicy(context.Background(), policy))
	due, err = st.GetDueScheduledWork(context.Background())
	require.NoError(t, err)
	require.Len(t, due, 1)

	// An empty assignment snapshot removes the prior policy locally without a
	// synthetic policy row.
	require.NoError(t, st.ReconcilePolicy(context.Background(), &pb.DesiredPolicy{Revision: "01K00000000000000000000015"}))
	due, err = st.GetDueScheduledWork(context.Background())
	require.NoError(t, err)
	require.Empty(t, due)
	var remaining int
	require.NoError(t, st.db.QueryRow(`SELECT COUNT(*) FROM scheduled_work WHERE retired = FALSE`).Scan(&remaining))
	require.Zero(t, remaining, "idle unassigned policy work is deleted, not left retired")
}

func TestReconcilePolicyRejectsDuplicateManifestIdentity(t *testing.T) {
	st, err := New(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, st.Close()) })

	manifest := testManifest()
	err = st.ReconcilePolicy(context.Background(), &pb.DesiredPolicy{
		Revision:  "01K00000000000000000000016",
		Manifests: []*pb.Manifest{manifest, manifest},
	})
	require.ErrorContains(t, err, "duplicate manifest identity")
}

func TestReconcilePolicyKeepsStoredManifestAcrossResealing(t *testing.T) {
	st, err := New(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, st.Close()) })

	manifest := testManifest()
	manifest.Occurrences[0].Action = &pb.Action{
		Id: &pb.ActionId{Value: "01K00000000000000000000004"}, Type: pb.ActionType_ACTION_TYPE_ENCRYPTION,
		Params: &pb.Action_Encryption{Encryption: &pb.EncryptionParams{
			PresharedKey: []byte("first-seal"), RotationIntervalDays: 30,
		}},
	}
	policy := &pb.DesiredPolicy{Revision: "01K00000000000000000000026", Manifests: []*pb.Manifest{manifest}}
	require.NoError(t, st.ReconcilePolicy(context.Background(), policy))

	manifest.Occurrences[0].Action.GetEncryption().PresharedKey = []byte("changed")
	require.NoError(t, st.ReconcilePolicy(context.Background(), policy))
	due, err := st.GetDueScheduledWork(context.Background())
	require.NoError(t, err)
	require.Len(t, due, 1)
	assert.Equal(t, []byte("first-seal"), due[0].Manifest.GetOccurrences()[0].GetAction().GetEncryption().GetPresharedKey())
}

func TestPolicyRunIdentityRotatesAndRetiredWorkDeletesAfterCompletion(t *testing.T) {
	st, err := New(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, st.Close()) })
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	st.SetClockForTest(func() time.Time { return now })
	manifest := testManifest()
	manifest.ManifestId = "01K00000000000000000000022"
	policy := &pb.DesiredPolicy{Revision: "01K000000000000000000000023", Manifests: []*pb.Manifest{manifest}}
	require.NoError(t, st.ReconcilePolicy(context.Background(), policy))
	due, err := st.GetDueScheduledWork(context.Background())
	require.NoError(t, err)
	require.Len(t, due, 1)
	workID := due[0].WorkID
	require.NotEmpty(t, workID)
	require.NoError(t, func() error {
		_, err := st.BeginManifestRun(context.Background(), &due[0], now)
		return err
	}())
	firstRun := due[0].RunID
	require.NotEqual(t, workID, firstRun, "a policy firing must have a distinct run identity")

	_, err = st.RecordManifestResult(context.Background(), &pb.ManifestResult{
		RunId:      firstRun,
		ManifestId: manifest.GetManifestId(),
	})
	require.NoError(t, err)
	now = now.Add(9 * time.Hour)
	st.SetClockForTest(func() time.Time { return now })
	due, err = st.GetDueScheduledWork(context.Background())
	require.NoError(t, err)
	require.Len(t, due, 1)
	require.NotEqual(t, firstRun, due[0].RunID, "recurring policy firing must receive a fresh run identity")
	secondRun := due[0].RunID
	_, err = st.BeginManifestRun(context.Background(), &due[0], now)
	require.NoError(t, err)
	require.NoError(t, st.ReconcilePolicy(context.Background(), &pb.DesiredPolicy{Revision: "01K000000000000000000000025"}))
	due, err = st.GetDueScheduledWork(context.Background())
	require.NoError(t, err)
	require.Len(t, due, 1, "retired work remains resumable while its run is active")
	require.True(t, due[0].RunInProgress)
	_, err = st.RecordManifestResult(context.Background(), &pb.ManifestResult{RunId: secondRun, ManifestId: manifest.GetManifestId()})
	require.NoError(t, err)
	var remaining int
	require.NoError(t, st.db.QueryRow(`SELECT COUNT(*) FROM scheduled_work WHERE work_id = ?`, workID).Scan(&remaining))
	require.Zero(t, remaining, "retired policy work is deleted after its active run closes")
}

func TestRecordManifestResultReturnsCleanupFailure(t *testing.T) {
	st, err := New(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, st.Close()) })
	manifest := testManifest()
	require.NoError(t, st.ReconcilePolicy(context.Background(), &pb.DesiredPolicy{
		Revision:  "01K00000000000000000000045",
		Manifests: []*pb.Manifest{manifest},
	}))
	due, err := st.GetDueScheduledWork(context.Background())
	require.NoError(t, err)
	require.Len(t, due, 1)
	require.NoError(t, func() error {
		_, err := st.BeginManifestRun(context.Background(), &due[0], time.Now())
		return err
	}())
	_, err = st.db.Exec(`
		CREATE TRIGGER fail_manifest_cleanup
		BEFORE UPDATE OF run_id ON scheduled_work
		WHEN NEW.run_id IS NULL
		BEGIN
			SELECT RAISE(ABORT, 'cleanup failed');
		END
	`)
	require.NoError(t, err)

	_, err = st.RecordManifestResult(context.Background(), &pb.ManifestResult{
		RunId:      due[0].RunID,
		ManifestId: manifest.GetManifestId(),
	})
	require.ErrorContains(t, err, "cleanup failed")
	var outbox int
	require.NoError(t, st.db.QueryRow(`SELECT COUNT(*) FROM result_outbox`).Scan(&outbox))
	assert.Zero(t, outbox)
}

func TestRecoverInterruptedOccurrenceQueuesIndeterminate(t *testing.T) {
	st, err := New(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, st.Close()) })
	manifest := testManifest()
	policy := &pb.DesiredPolicy{Revision: "01K00000000000000000000035", Manifests: []*pb.Manifest{manifest}}
	require.NoError(t, st.ReconcilePolicy(context.Background(), policy))
	due, err := st.GetDueScheduledWork(context.Background())
	require.NoError(t, err)
	require.Len(t, due, 1)
	_, err = st.BeginManifestRun(context.Background(), &due[0], time.Now())
	require.NoError(t, err)
	occurrence := manifest.GetOccurrences()[0]
	require.NoError(t, st.MarkOccurrenceStarted(context.Background(), due[0].RunID, occurrence.GetOccurrenceId(), time.Now()))

	_, err = st.RecoverInterruptedOccurrences(context.Background())
	require.NoError(t, err)
	pending, err := st.GetPendingResults(context.Background())
	require.NoError(t, err)
	require.Len(t, pending, 1)
	require.Equal(t, pb.ExecutionStatus_EXECUTION_STATUS_INDETERMINATE, pending[0].ActionResult.GetStatus())
	require.Equal(t, due[0].RunID, pending[0].ActionResult.GetRunId().GetValue())
	require.Equal(t, occurrence.GetOccurrenceId(), pending[0].ActionResult.GetOccurrenceId().GetValue())

	_, err = st.RecoverInterruptedOccurrences(context.Background())
	require.NoError(t, err)
	pending, err = st.GetPendingResults(context.Background())
	require.NoError(t, err)
	require.Len(t, pending, 1, "recovery must be idempotent")
}
