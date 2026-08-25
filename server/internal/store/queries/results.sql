-- name: GetOSQueryResult :one
SELECT query_id, device_id, completed, success, error, rows, created_at
FROM osquery_results
WHERE query_id = ?;

-- name: InsertPendingOSQueryResult :exec
INSERT INTO osquery_results (
    query_id, device_id, table_name, completed, success, error, rows, created_at
) VALUES (
    sqlc.arg(query_id), sqlc.arg(device_id), sqlc.arg(table_name),
    FALSE, FALSE, '', '[]', sqlc.arg(created_at)
);
-- name: FailPendingOSQueryResult :execrows
UPDATE osquery_results
SET completed = TRUE, success = FALSE, error = sqlc.arg(error),
    rows = '[]', completed_at = sqlc.arg(completed_at)
WHERE query_id = sqlc.arg(query_id)
  AND device_id = sqlc.arg(device_id)
  AND completed = FALSE;

-- name: CompleteOSQueryResult :execrows
UPDATE osquery_results
SET completed = TRUE, success = sqlc.arg(success), error = sqlc.arg(error),
    rows = sqlc.arg(rows), completed_at = sqlc.arg(completed_at)
WHERE query_id = sqlc.arg(query_id)
  AND device_id = sqlc.arg(device_id)
  AND completed = FALSE;

-- name: GetDeviceLogResult :one
SELECT query_id, device_id, completed, success, error, logs, created_at
FROM log_query_results
WHERE query_id = ?;

-- name: InsertPendingLogQueryResult :exec
INSERT INTO log_query_results (
    query_id, device_id, completed, success, error, logs, created_at
) VALUES (
    sqlc.arg(query_id), sqlc.arg(device_id), FALSE, FALSE, '', '', sqlc.arg(created_at)
);

-- name: FailPendingLogQueryResult :execrows
UPDATE log_query_results
SET completed = TRUE, success = FALSE, error = sqlc.arg(error),
    logs = '', completed_at = sqlc.arg(completed_at)
WHERE query_id = sqlc.arg(query_id)
  AND device_id = sqlc.arg(device_id)
  AND completed = FALSE;

-- name: CompleteLogQueryResult :execrows
UPDATE log_query_results
SET completed = TRUE, success = sqlc.arg(success), error = sqlc.arg(error),
    logs = sqlc.arg(logs), completed_at = sqlc.arg(completed_at)
WHERE query_id = sqlc.arg(query_id)
  AND device_id = sqlc.arg(device_id)
  AND completed = FALSE;

-- name: UpsertDeviceInventoryTable :exec
INSERT INTO device_inventory (device_id, table_name, rows, collected_at)
VALUES (sqlc.arg(device_id), sqlc.arg(table_name), sqlc.arg(rows), sqlc.arg(collected_at))
ON CONFLICT (device_id, table_name) DO UPDATE
SET rows = EXCLUDED.rows, collected_at = EXCLUDED.collected_at;

-- name: ListDeviceComplianceResults :many
SELECT cr.action_id, a.name AS action_name, cr.compliant,
       cr.detection_output, cr.checked_at
FROM compliance_results cr
JOIN actions a ON a.id = cr.action_id AND a.is_deleted = FALSE
WHERE cr.device_id = ?
ORDER BY cr.action_id;

-- name: ListDeviceComplianceEvaluations :many
SELECT e.policy_id, p.name AS policy_name,
       e.action_id, a.name AS action_name,
       e.status, e.compliant, r.grace_period_hours,
       e.checked_at, e.first_failed_at, cr.detection_output
FROM compliance_policy_evaluation e
JOIN compliance_policies p ON p.id = e.policy_id AND p.is_deleted = FALSE
JOIN compliance_policy_rules r ON r.policy_id = e.policy_id AND r.action_id = e.action_id
JOIN actions a ON a.id = e.action_id AND a.is_deleted = FALSE
LEFT JOIN compliance_results cr ON cr.device_id = e.device_id AND cr.action_id = e.action_id
WHERE e.device_id = ?
ORDER BY e.policy_id, e.action_id;





-- name: UpsertDeviceComplianceResult :execrows
INSERT INTO compliance_results (
    device_id, action_id, action_name, compliant, detection_output, checked_at
)
SELECT sqlc.arg(device_id), a.id, a.name, sqlc.arg(compliant),
       sqlc.narg(detection_output), sqlc.arg(checked_at)
FROM actions a
WHERE a.id = sqlc.arg(action_id) AND a.is_deleted = FALSE
ON CONFLICT (device_id, action_id) DO UPDATE
SET action_name = excluded.action_name,
    compliant = excluded.compliant,
    detection_output = excluded.detection_output,
    checked_at = excluded.checked_at;

-- name: ListComplianceRuleEvaluationTargets :many
SELECT r.policy_id, r.grace_period_hours, e.first_failed_at
FROM compliance_policy_rules r
JOIN compliance_policies p ON p.id = r.policy_id AND p.is_deleted = FALSE
LEFT JOIN compliance_policy_evaluation e
       ON e.policy_id = r.policy_id AND e.action_id = r.action_id
      AND e.device_id = sqlc.arg(device_id)
WHERE r.action_id = sqlc.arg(action_id)
ORDER BY r.policy_id;

-- name: UpsertCompliancePolicyEvaluation :exec
INSERT INTO compliance_policy_evaluation (
    device_id, policy_id, action_id, compliant, first_failed_at, status, checked_at
) VALUES (
    sqlc.arg(device_id), sqlc.arg(policy_id), sqlc.arg(action_id),
    sqlc.arg(compliant), sqlc.narg(first_failed_at), sqlc.arg(status), sqlc.arg(checked_at)
)
ON CONFLICT (device_id, policy_id, action_id) DO UPDATE
SET compliant = excluded.compliant,
    first_failed_at = excluded.first_failed_at,
    status = excluded.status,
    checked_at = excluded.checked_at;








-- name: RefreshDeviceComplianceStatus :execrows
UPDATE devices
SET compliance_total = (
        SELECT COUNT(*)
        FROM compliance_results cr
        JOIN actions a ON a.id = cr.action_id AND a.is_deleted = FALSE
        WHERE cr.device_id = sqlc.arg(device_id)
    ),
    compliance_passing = (
        SELECT COUNT(*)
        FROM compliance_results cr
        JOIN actions a ON a.id = cr.action_id AND a.is_deleted = FALSE
        WHERE cr.device_id = sqlc.arg(device_id) AND cr.compliant
    ),
    compliance_checked_at = CASE
        WHEN NOT EXISTS (
            SELECT 1
            FROM compliance_results cr
            JOIN actions a ON a.id = cr.action_id AND a.is_deleted = FALSE
            WHERE cr.device_id = sqlc.arg(device_id)
        ) THEN NULL
        WHEN compliance_checked_at IS NULL OR compliance_checked_at < sqlc.arg(checked_at)
        THEN sqlc.arg(checked_at)
        ELSE compliance_checked_at
    END,
    compliance_status = (
        SELECT CASE
                   WHEN COUNT(*) = 0 THEN 0
                   WHEN MAX(severity) = 3 THEN 2
                   WHEN MAX(severity) = 2 THEN 3
                   WHEN MAX(severity) = 1 THEN 0
                   ELSE 1
               END
        FROM (
            SELECT CASE e.status WHEN 2 THEN 3 WHEN 3 THEN 2 WHEN 0 THEN 1 ELSE 0 END AS severity
            FROM compliance_policy_evaluation e
            JOIN compliance_policies p ON p.id = e.policy_id AND p.is_deleted = FALSE
            JOIN compliance_policy_rules r ON r.policy_id = e.policy_id AND r.action_id = e.action_id
            JOIN actions a ON a.id = e.action_id AND a.is_deleted = FALSE
            WHERE e.device_id = sqlc.arg(device_id)
            UNION ALL
            SELECT CASE WHEN cr.compliant THEN 0 ELSE 3 END
            FROM compliance_results cr
            JOIN actions a ON a.id = cr.action_id AND a.is_deleted = FALSE
            WHERE cr.device_id = sqlc.arg(device_id)
              AND NOT EXISTS (
                  SELECT 1
                  FROM compliance_policy_rules r
                  JOIN compliance_policies p ON p.id = r.policy_id AND p.is_deleted = FALSE
                  WHERE r.action_id = cr.action_id
              )
        )
    )
WHERE id = sqlc.arg(device_id) AND is_deleted = FALSE;

-- name: GetPolicyActionResult :one
SELECT result_hash, device_id
FROM policy_action_results
WHERE run_id = sqlc.arg(run_id) AND occurrence_id = sqlc.arg(occurrence_id);

-- name: InsertPolicyActionResult :exec
INSERT INTO policy_action_results (
    run_id, occurrence_id, device_id, action_id, result_hash, payload, created_at
) VALUES (
    sqlc.arg(run_id), sqlc.arg(occurrence_id), sqlc.arg(device_id), sqlc.arg(action_id),
    sqlc.arg(result_hash), sqlc.arg(payload), sqlc.arg(created_at)
);

-- name: GetPolicyManifestResult :one
SELECT state, result_code, device_id, manifest_id
FROM policy_manifest_results
WHERE run_id = ?;

-- name: InsertPolicyManifestResult :exec
INSERT INTO policy_manifest_results (
    run_id, device_id, manifest_id, state, result_code, created_at
) VALUES (
    sqlc.arg(run_id), sqlc.arg(device_id), sqlc.arg(manifest_id), sqlc.arg(state),
    sqlc.arg(result_code), sqlc.arg(created_at)
);
