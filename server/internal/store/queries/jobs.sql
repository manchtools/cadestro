

-- name: InsertJob :one
INSERT INTO jobs (job_id, kind, payload, state, due_at, max_attempts, dedupe_key)
VALUES (?, ?, ?, 'PENDING', ?, ?, ?)
RETURNING *;

-- name: GetJob :one
SELECT * FROM jobs WHERE job_id = ?;

-- name: GetLiveJobByDedupe :one
SELECT * FROM jobs
WHERE dedupe_key = ? AND state IN ('PENDING', 'CLAIMED')
LIMIT 1;

-- name: ClaimJob :execrows




UPDATE jobs
SET state = 'CLAIMED',
    claimed_at = sqlc.arg(now),
    claimed_until = sqlc.arg(claimed_until),
    claimed_by = sqlc.arg(claimed_by),
    attempt_count = attempt_count + 1,
    updated_at = sqlc.arg(now)
WHERE job_id = sqlc.arg(job_id)
  AND (
        (state = 'PENDING' AND due_at <= sqlc.arg(now))
     OR (state = 'CLAIMED' AND claimed_until <= sqlc.arg(now))
  );

-- name: ListClaimableJobs :many


SELECT * FROM jobs
WHERE (state = 'PENDING' AND due_at <= sqlc.arg(now))
   OR (state = 'CLAIMED' AND claimed_until <= sqlc.arg(now))
ORDER BY due_at
LIMIT sqlc.arg(page_size);

-- name: ReleaseJobClaim :execrows

UPDATE jobs
SET state = 'PENDING',
    claimed_at = NULL,
    claimed_until = NULL,
    claimed_by = '',
    due_at = ?,
    result_code = ?,
    updated_at = ?
WHERE job_id = ?
  AND state = 'CLAIMED'
  AND claimed_by = ?;

-- name: FinishJob :execrows
UPDATE jobs
SET state = sqlc.arg(new_state),
    claimed_at = NULL,
    claimed_until = NULL,
    claimed_by = '',
    terminal_at = sqlc.arg(terminal_at),
    result_code = sqlc.arg(result_code),
    updated_at = sqlc.arg(updated_at)
WHERE job_id = sqlc.arg(job_id)
  AND state = 'CLAIMED'
  AND claimed_by = sqlc.arg(claimed_by);

-- name: RescheduleJob :execrows


UPDATE jobs
SET state = 'PENDING',
    due_at = ?,
    claimed_at = NULL,
    claimed_until = NULL,
    claimed_by = '',
    attempt_count = 0,
    result_code = 'OK',
    updated_at = ?
WHERE job_id = ?
  AND state = 'CLAIMED'
  AND claimed_by = ?;

-- name: CancelPendingJob :execrows
UPDATE jobs
SET state = 'CANCELLED',
    terminal_at = ?,
    result_code = ?,
    updated_at = ?
WHERE job_id = ?
  AND state = 'PENDING';

-- name: DeleteTerminalJobsBefore :execrows


DELETE FROM jobs
WHERE terminal_at IS NOT NULL AND terminal_at < ?;
