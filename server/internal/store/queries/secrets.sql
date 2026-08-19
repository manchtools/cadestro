-- Current and bounded historical device-secret metadata. List queries never
-- select ciphertext; one-entry reveal queries are the only administrative
-- read path for stored secret values.

-- name: GetDeviceSecret :one
SELECT id, device_id, kind, subject, version, ciphertext
FROM device_secrets
WHERE id = ?;

-- name: InsertDeviceSecret :exec
INSERT INTO device_secrets (id, device_id, kind, subject, version, ciphertext)
VALUES (?, ?, ?, ?, ?, ?);

-- name: ListCurrentLpsPasswords :many
SELECT p.id, ds.device_id, d.hostname AS device_hostname,
       ds.subject AS action_id, COALESCE(a.name, '') AS action_name,
       p.username, p.rotated_at, p.rotation_reason
FROM lps_passwords p
JOIN device_secrets ds ON ds.id = p.id AND ds.kind = 'lps'
JOIN devices d ON d.id = ds.device_id AND d.is_deleted = FALSE
LEFT JOIN actions a ON a.id = ds.subject AND a.is_deleted = FALSE
WHERE ds.device_id = ? AND p.is_current = TRUE
ORDER BY ds.subject, p.username, p.id;

-- name: ListLpsPasswordHistory :many
SELECT id, device_id, device_hostname, action_id, action_name,
       username, rotated_at, rotation_reason
FROM (
       SELECT p.id, ds.device_id, d.hostname AS device_hostname,
           ds.subject AS action_id, COALESCE(a.name, '') AS action_name,
           p.username, p.rotated_at, p.rotation_reason,
           row_number() OVER (
               PARTITION BY ds.subject
               ORDER BY p.rotated_at DESC, p.id DESC
           ) AS history_position
    FROM lps_passwords p
    JOIN device_secrets ds ON ds.id = p.id AND ds.kind = 'lps'
    JOIN devices d ON d.id = ds.device_id AND d.is_deleted = FALSE
    LEFT JOIN actions a ON a.id = ds.subject AND a.is_deleted = FALSE
    WHERE ds.device_id = ? AND p.is_current = FALSE
) ranked
WHERE history_position <= 3
ORDER BY rotated_at DESC, id DESC;

-- name: GetLpsPasswordForReveal :one
SELECT p.id, ds.device_id, ds.subject AS action_id
FROM lps_passwords p JOIN device_secrets ds ON ds.id = p.id AND ds.kind = 'lps'
WHERE p.id = ?;

-- name: InsertLuksToken :one
INSERT INTO luks_tokens (
    id, device_id, action_id, token, min_length, complexity, created_at, expires_at
) VALUES (
    sqlc.arg(id), sqlc.arg(device_id), sqlc.arg(action_id), sqlc.arg(token),
    sqlc.arg(min_length), sqlc.arg(complexity), sqlc.arg(created_at), sqlc.arg(expires_at)
)
RETURNING *;

-- name: ConsumeLuksToken :one
UPDATE luks_tokens
SET used = TRUE
WHERE token = sqlc.arg(token)
  AND device_id = sqlc.arg(device_id)
  AND used = FALSE
  AND expires_at > sqlc.arg(now)
RETURNING *;

-- name: GetCurrentLuksKeyForAgent :one
SELECT k.id, ds.device_id, ds.subject AS action_id, k.device_path, k.rotated_at,
       k.rotation_reason, k.is_current, k.created_at, k.revocation_status,
       k.revocation_error, k.revocation_at
FROM luks_keys k JOIN device_secrets ds ON ds.id = k.id AND ds.kind = 'luks'
WHERE ds.device_id = sqlc.arg(device_id)
  AND ds.subject = sqlc.arg(action_id)
  AND k.is_current = TRUE
ORDER BY k.rotated_at DESC, k.id DESC
LIMIT 1;

-- name: RetireCurrentLuksKeys :execrows
UPDATE luks_keys SET is_current = FALSE
WHERE id IN (SELECT k.id FROM luks_keys k JOIN device_secrets ds ON ds.id = k.id AND ds.kind = 'luks'
             WHERE ds.device_id = sqlc.arg(device_id) AND ds.subject = sqlc.arg(action_id))
  AND is_current = TRUE;

-- name: InsertLuksKey :one
INSERT INTO luks_keys (
    id, device_path,
    rotated_at, rotation_reason, created_at
) VALUES (
    sqlc.arg(id),
    sqlc.arg(device_path), sqlc.arg(rotated_at),
    sqlc.arg(rotation_reason), sqlc.arg(created_at)
)
RETURNING *;

-- name: RetireCurrentLpsPassword :execrows
UPDATE lps_passwords SET is_current = FALSE
WHERE id IN (SELECT p.id FROM lps_passwords p JOIN device_secrets ds ON ds.id = p.id AND ds.kind = 'lps'
             WHERE ds.device_id = sqlc.arg(device_id) AND ds.subject = sqlc.arg(action_id)
               AND p.username = sqlc.arg(username))
  AND is_current = TRUE;

-- name: InsertLpsPassword :one
INSERT INTO lps_passwords (
    id, username,
    rotated_at, rotation_reason, created_at
) VALUES (
       sqlc.arg(id), sqlc.arg(username),
       sqlc.arg(rotated_at),
    sqlc.arg(rotation_reason), sqlc.arg(created_at)
)
RETURNING *;

-- name: ListCurrentLuksKeys :many
SELECT k.id, ds.device_id, d.hostname AS device_hostname,
       ds.subject AS action_id, COALESCE(a.name, '') AS action_name,
       k.device_path, k.rotated_at, k.rotation_reason,
       k.revocation_status, k.revocation_error, k.revocation_at
FROM luks_keys k
JOIN device_secrets ds ON ds.id = k.id AND ds.kind = 'luks'
JOIN devices d ON d.id = ds.device_id AND d.is_deleted = FALSE
LEFT JOIN actions a ON a.id = ds.subject AND a.is_deleted = FALSE
WHERE ds.device_id = ? AND k.is_current = TRUE
ORDER BY ds.subject, k.device_path, k.id;

-- name: ListLuksKeyHistory :many
SELECT id, device_id, device_hostname, action_id, action_name,
       device_path, rotated_at, rotation_reason,
       revocation_status, revocation_error, revocation_at
FROM (
    SELECT k.id, ds.device_id, d.hostname AS device_hostname,
           ds.subject AS action_id, COALESCE(a.name, '') AS action_name,
           k.device_path, k.rotated_at, k.rotation_reason,
           k.revocation_status, k.revocation_error, k.revocation_at,
           row_number() OVER (
               PARTITION BY ds.subject
               ORDER BY k.rotated_at DESC, k.id DESC
           ) AS history_position
    FROM luks_keys k
    JOIN device_secrets ds ON ds.id = k.id AND ds.kind = 'luks'
    JOIN devices d ON d.id = ds.device_id AND d.is_deleted = FALSE
    LEFT JOIN actions a ON a.id = ds.subject AND a.is_deleted = FALSE
    WHERE ds.device_id = ? AND k.is_current = FALSE
) ranked
WHERE history_position <= 3
ORDER BY rotated_at DESC, id DESC;

-- name: GetLuksKeyForReveal :one
SELECT k.id, ds.device_id, ds.subject AS action_id
FROM luks_keys k JOIN device_secrets ds ON ds.id = k.id AND ds.kind = 'luks'
WHERE k.id = ?;

-- name: GetLuksRevocationTarget :one
SELECT count(*) AS key_count,
       EXISTS (
           SELECT 1 FROM luks_keys pending JOIN device_secrets pds ON pds.id = pending.id AND pds.kind = 'luks'
           WHERE pds.device_id = sqlc.arg(device_id)
             AND pds.subject = sqlc.arg(action_id)
             AND pending.is_current = TRUE
             AND pending.revocation_status = 'dispatched'
       ) AS dispatch_pending,
       EXISTS (
           SELECT 1 FROM luks_keys revoked JOIN device_secrets rds ON rds.id = revoked.id AND rds.kind = 'luks'
           WHERE rds.device_id = sqlc.arg(device_id)
             AND rds.subject = sqlc.arg(action_id)
             AND revoked.is_current = TRUE
             AND revoked.revocation_status = 'success'
       ) AS already_revoked
FROM luks_keys k JOIN device_secrets ds ON ds.id = k.id AND ds.kind = 'luks'
WHERE ds.device_id = sqlc.arg(device_id)
  AND ds.subject = sqlc.arg(action_id)
  AND is_current = TRUE;

-- name: MarkLuksKeyRevocationDispatched :execrows
UPDATE luks_keys
SET revocation_status = 'dispatched',
    revocation_error = NULL,
    revocation_at = sqlc.arg(revocation_at)
WHERE id IN (SELECT k.id FROM luks_keys k JOIN device_secrets ds ON ds.id = k.id AND ds.kind = 'luks'
             WHERE ds.device_id = sqlc.arg(device_id) AND ds.subject = sqlc.arg(action_id))
  AND is_current = TRUE
  AND COALESCE(revocation_status, '') NOT IN ('dispatched', 'success');

-- name: MarkLuksKeyRevocationDispatchFailed :execrows
UPDATE luks_keys
SET revocation_status = 'failed',
    revocation_error = 'device unavailable',
    revocation_at = sqlc.arg(revocation_at)
WHERE id IN (SELECT k.id FROM luks_keys k JOIN device_secrets ds ON ds.id = k.id AND ds.kind = 'luks'
             WHERE ds.device_id = sqlc.arg(device_id) AND ds.subject = sqlc.arg(action_id))
  AND is_current = TRUE
  AND revocation_status = 'dispatched';

-- name: CompleteLuksKeyRevocation :execrows
UPDATE luks_keys
SET revocation_status = sqlc.arg(revocation_status),
    revocation_error = sqlc.narg(revocation_error),
    revocation_at = sqlc.arg(revocation_at)
WHERE id IN (SELECT k.id FROM luks_keys k JOIN device_secrets ds ON ds.id = k.id AND ds.kind = 'luks'
             WHERE ds.device_id = sqlc.arg(device_id) AND ds.subject = sqlc.arg(action_id))
  AND is_current = TRUE
  AND revocation_status = 'dispatched';
