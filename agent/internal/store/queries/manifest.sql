-- name: ResolveWorkID :one
SELECT work_id FROM scheduled_work WHERE work_id = ? OR COALESCE(run_id, '') = ?;

-- name: GetAssignedPolicyRevision :one
SELECT value FROM settings WHERE key = 'assigned_policy_revision';

-- name: ListActiveWork :many
SELECT work_id, run_in_progress
FROM scheduled_work
WHERE retired = FALSE;

-- name: RetireWork :exec
UPDATE scheduled_work SET retired = TRUE WHERE work_id = ?;

-- name: DeleteWork :exec
DELETE FROM scheduled_work WHERE work_id = ?;

-- name: ScheduledWorkExists :one
SELECT 1 FROM scheduled_work WHERE work_id = ?;

-- name: ReviveWork :exec
UPDATE scheduled_work SET retired = FALSE WHERE work_id = ?;

-- name: InsertScheduledWork :exec
INSERT INTO scheduled_work
    (work_id, run_id, manifest_blob, retired, received_at, next_execute_at)
VALUES (?, ?, ?, FALSE, ?, ?);

-- name: InsertOccurrence :exec
INSERT INTO scheduled_work_occurrences
    (work_id, occurrence_id, position, action_id)
VALUES (?, ?, ?, ?);

-- name: SetAssignedPolicyRevision :exec
INSERT INTO settings (key, value) VALUES ('assigned_policy_revision', ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value;

-- name: GetDueScheduledWork :many
SELECT work_id, COALESCE(run_id, ''), manifest_blob,
       received_at, last_executed_at, next_execute_at,
       run_started_at, run_in_progress
FROM scheduled_work
WHERE (retired = FALSE OR run_in_progress = TRUE)
  AND (run_in_progress = TRUE OR next_execute_at <= ?)
ORDER BY run_in_progress DESC, next_execute_at, work_id;

-- name: GetManifestActions :many
SELECT work_id, manifest_blob, received_at, last_executed_at, next_execute_at
FROM scheduled_work
WHERE retired = FALSE
ORDER BY received_at, work_id;

-- name: GetScheduledRun :one
SELECT work_id, COALESCE(run_id, ''), run_in_progress, run_started_at
FROM scheduled_work
WHERE (work_id = ? OR COALESCE(run_id, '') = ?)
  AND (retired = FALSE OR run_in_progress = TRUE);

-- name: CountStartedOccurrences :one
SELECT COUNT(*)
FROM scheduled_work_occurrences
WHERE work_id = ? AND state = ?;

-- name: ResetOccurrences :exec
UPDATE scheduled_work_occurrences
SET state = ?, started_at = NULL, completed_at = NULL,
    result_status = NULL, result_error = ''
WHERE work_id = ?;

-- name: BeginScheduledRun :exec
UPDATE scheduled_work
SET run_id = ?, last_executed_at = ?, next_execute_at = ?,
    run_started_at = ?, run_in_progress = TRUE
WHERE work_id = ?;

-- name: MarkOccurrenceStarted :execrows
UPDATE scheduled_work_occurrences
SET state = ?, started_at = ?, completed_at = NULL
WHERE work_id = ? AND occurrence_id = ? AND state = ?;

-- name: GetOccurrenceStates :many
SELECT occurrence_id, state, result_status, result_error
FROM scheduled_work_occurrences
WHERE work_id = ?
ORDER BY position;

-- name: GetStartedOccurrenceHash :one
SELECT last_result_hash
FROM scheduled_work_occurrences
WHERE work_id = ? AND occurrence_id = ? AND state = ?;

-- name: RecordOccurrence :execrows
UPDATE scheduled_work_occurrences
SET state = ?, completed_at = ?, result_status = ?, result_error = ?, last_result_hash = ?
WHERE work_id = ? AND occurrence_id = ? AND state = ?;

-- name: FinishManifestRun :execrows
UPDATE scheduled_work
SET run_in_progress = FALSE, run_started_at = NULL
WHERE (work_id = ? OR COALESCE(run_id, '') = ?) AND run_in_progress = TRUE;

-- name: DeleteRetiredWork :exec
DELETE FROM scheduled_work
WHERE (work_id = ? OR COALESCE(run_id, '') = ?) AND retired = TRUE;

-- name: ClearRunID :exec
UPDATE scheduled_work SET run_id = NULL WHERE (work_id = ? OR COALESCE(run_id, '') = ?);

-- name: InsertResultOutbox :exec
INSERT INTO result_outbox (id, kind, payload, created_at)
VALUES (?, ?, ?, ?);

-- name: GetPendingResults :many
SELECT id, kind, payload
FROM result_outbox
WHERE synced = FALSE
ORDER BY sequence;

-- name: MarkPendingResultSynced :exec
UPDATE result_outbox SET synced = TRUE WHERE id = ?;

-- name: ListInterruptedOccurrences :many
SELECT COALESCE(sw.run_id, o.work_id), o.work_id, o.occurrence_id, o.action_id
FROM scheduled_work_occurrences o
JOIN scheduled_work sw ON sw.work_id = o.work_id
WHERE o.state = ?
ORDER BY o.work_id, o.position;

-- name: InsertRecoveredResult :exec
INSERT INTO result_outbox (id, kind, payload, created_at)
VALUES (?, 'ACTION', ?, ?);

-- name: RecoverOccurrence :exec
UPDATE scheduled_work_occurrences
SET state = ?, completed_at = ?, result_status = ?, result_error = ?
WHERE work_id = ? AND occurrence_id = ? AND state = ?;
