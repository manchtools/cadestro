package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/manchtools/cadestro/server/internal/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedDevice(t *testing.T, pool *testdb.DB) string {
	t.Helper()
	id := newID()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO devices (id, hostname) VALUES ($1, $2)`, id, "dispatch-"+id)
	require.NoError(t, err)
	return id
}

func TestJobs_ConditionalClaimAdmitsExactlyOneWorker(t *testing.T) {
	_, pool := setupSQLite(t)
	ctx := context.Background()
	now := time.Now().UTC()

	id := newID()
	workerA, workerB := newID(), newID()
	_, err := pool.Exec(ctx, `INSERT INTO jobs (job_id, kind, state, due_at)
		VALUES ($1, 'dynamic_group.evaluate', 'PENDING', $2)`, id, now.Add(-time.Minute))
	require.NoError(t, err)

	claim := `UPDATE jobs
		SET state = 'CLAIMED', claimed_at = $2, claimed_until = $3, claimed_by = $4,
		    attempt_count = attempt_count + 1, updated_at = $2
		WHERE job_id = $1
		  AND ((state = 'PENDING' AND due_at <= $2) OR (state = 'CLAIMED' AND claimed_until <= $2))`

	tag, err := pool.Exec(ctx, claim, id, now, now.Add(time.Minute), workerA)
	require.NoError(t, err)
	require.Equal(t, int64(1), tag.RowsAffected())

	tag, err = pool.Exec(ctx, claim, id, now, now.Add(time.Minute), workerB)
	require.NoError(t, err)
	assert.Zero(t, tag.RowsAffected(), "a live claim must not be stealable")

	later := now.Add(2 * time.Minute)
	tag, err = pool.Exec(ctx, claim, id, later, later.Add(time.Minute), workerB)
	require.NoError(t, err)
	assert.Equal(t, int64(1), tag.RowsAffected(), "an expired lease must be reclaimable")

	var attempts int32
	var by string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT attempt_count, claimed_by FROM jobs WHERE job_id = $1`, id).Scan(&attempts, &by))
	assert.Equal(t, int32(2), attempts)
	assert.Equal(t, workerB, by)
}

func TestJobs_DedupeKeyAdmitsOneLiveRow(t *testing.T) {
	_, pool := setupSQLite(t)
	ctx := context.Background()
	now := time.Now().UTC()

	insert := `INSERT INTO jobs (job_id, kind, state, due_at, dedupe_key)
		VALUES ($1, 'retention.sweep', 'PENDING', $2, 'retention.sweep')`

	first := newID()
	_, err := pool.Exec(ctx, insert, first, now)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, insert, newID(), now)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "jobs.dedupe_key")

	_, err = pool.Exec(ctx, `UPDATE jobs SET state = 'SUCCEEDED', terminal_at = $2 WHERE job_id = $1`, first, now)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, insert, newID(), now)
	require.NoError(t, err, "a terminal run must not block the next one")
}

func TestJobs_StateMachineRequiresTerminalTimestamps(t *testing.T) {
	_, pool := setupSQLite(t)
	ctx := context.Background()
	now := time.Now().UTC()

	_, err := pool.Exec(ctx, `INSERT INTO jobs (job_id, kind, state, due_at)
		VALUES ($1, 'retention.sweep', 'SUCCEEDED', $2)`, newID(), now)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CASE state")

	_, err = pool.Exec(ctx, `INSERT INTO jobs (job_id, kind, state, due_at, claimed_at)
		VALUES ($1, 'retention.sweep', 'CLAIMED', $2, $2)`, newID(), now)
	require.Error(t, err, "a claim needs both a start and a lease expiry")
	assert.Contains(t, err.Error(), "claimed_at IS NULL")
}
