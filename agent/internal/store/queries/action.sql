-- name: GetAssignedPolicyRevision :one
SELECT value FROM settings WHERE key = 'assigned_policy_revision';
-- name: ListAllWork :many
SELECT work_id, action_blob, retired, run_in_progress FROM scheduled_work;
-- name: RetireWork :exec
UPDATE scheduled_work SET retired = TRUE, updated_at = CURRENT_TIMESTAMP WHERE work_id = ?;
-- name: DeleteWork :exec
DELETE FROM scheduled_work WHERE work_id = ?;
-- name: UpdateScheduledWork :exec
UPDATE scheduled_work SET action_blob = ?, retired = FALSE, next_execute_at = ?, updated_at = CURRENT_TIMESTAMP WHERE work_id = ?;
-- name: InsertScheduledWork :exec
INSERT INTO scheduled_work (work_id, run_id, action_blob, retired, received_at, next_execute_at, created_at, updated_at)
VALUES (?, NULL, ?, FALSE, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
-- name: SetAssignedPolicyRevision :exec
INSERT INTO settings (key, value, created_at, updated_at) VALUES ('assigned_policy_revision', ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP;
-- name: GetDueScheduledWork :many
SELECT work_id, COALESCE(run_id, ''), action_blob
FROM scheduled_work WHERE (retired = FALSE OR run_in_progress = TRUE) AND (run_in_progress = TRUE OR next_execute_at <= ?)
ORDER BY run_in_progress DESC, next_execute_at, work_id;
-- name: GetScheduledWorkNextExecute :one
SELECT next_execute_at FROM scheduled_work WHERE work_id = ?;
-- name: GetScheduledRun :one
SELECT work_id, COALESCE(run_id, ''), run_in_progress, run_started_at FROM scheduled_work WHERE (work_id = ? OR run_id = ?) AND (retired = FALSE OR run_in_progress = TRUE);
-- name: BeginScheduledRun :execrows
UPDATE scheduled_work SET run_id = ?, last_executed_at = ?, next_execute_at = ?, run_started_at = ?, run_in_progress = TRUE, updated_at = CURRENT_TIMESTAMP
WHERE work_id = ? AND run_in_progress = FALSE;
-- name: FinishScheduledRun :one
UPDATE scheduled_work SET run_in_progress = FALSE, run_started_at = NULL, run_id = CASE WHEN retired THEN run_id ELSE NULL END, updated_at = CURRENT_TIMESTAMP
WHERE (work_id = ? OR run_id = ?) AND run_in_progress = TRUE RETURNING retired;
-- name: DeleteRetiredWork :exec
DELETE FROM scheduled_work WHERE (work_id = ? OR run_id = ?) AND retired = TRUE;
-- name: InsertResultOutbox :one
INSERT INTO result_outbox (payload, created_at) VALUES (?, CURRENT_TIMESTAMP) RETURNING sequence;
-- name: GetPendingResults :many
SELECT sequence, payload FROM result_outbox ORDER BY sequence;
-- name: DeletePendingResult :exec
DELETE FROM result_outbox WHERE sequence = ?;
-- name: ListInterruptedWork :many
SELECT work_id, COALESCE(run_id, ''), action_blob, run_started_at FROM scheduled_work WHERE run_in_progress = TRUE ORDER BY work_id;
