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
-- name: GetDueScheduledWork :many
SELECT work_id, action_blob FROM scheduled_work
WHERE retired = FALSE AND run_in_progress = FALSE AND next_execute_at <= ?
ORDER BY next_execute_at, work_id;
-- name: GetScheduledWorkNextExecute :one
SELECT next_execute_at FROM scheduled_work WHERE work_id = ?;
-- name: GetRunnableWork :one
SELECT work_id, action_blob FROM scheduled_work
WHERE work_id = ? AND retired = FALSE AND run_in_progress = FALSE AND next_execute_at <= ?;
-- name: BeginScheduledRun :execrows
UPDATE scheduled_work SET run_id = ?, run_action_digest = ?, last_executed_at = ?, next_execute_at = ?, run_started_at = ?, run_in_progress = TRUE, updated_at = CURRENT_TIMESTAMP
WHERE work_id = ? AND retired = FALSE AND run_in_progress = FALSE AND next_execute_at <= ?;
-- name: FinishScheduledRun :one
UPDATE scheduled_work SET run_in_progress = FALSE, run_started_at = NULL, run_action_digest = NULL, run_id = CASE WHEN retired THEN run_id ELSE NULL END, updated_at = CURRENT_TIMESTAMP
WHERE work_id = ? AND run_id = ? AND run_action_digest = ? AND run_in_progress = TRUE RETURNING retired;
-- name: DeleteRetiredWork :exec
DELETE FROM scheduled_work WHERE work_id = ? AND run_id = ? AND retired = TRUE;
-- name: InsertResultOutbox :one
INSERT INTO result_outbox (payload, created_at) VALUES (?, CURRENT_TIMESTAMP) RETURNING sequence;
-- name: GetPendingResults :many
SELECT sequence, payload FROM result_outbox ORDER BY sequence;
-- name: DeletePendingResult :exec
DELETE FROM result_outbox WHERE sequence = ?;
-- name: ListInterruptedWork :many
SELECT work_id, COALESCE(run_id, ''), run_action_digest, run_started_at FROM scheduled_work WHERE run_in_progress = TRUE ORDER BY work_id;
