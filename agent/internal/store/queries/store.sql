-- name: GetLuksState :one
SELECT action_id, device_path, ownership_taken, device_key_type, last_rotated_at
FROM luks_state
WHERE action_id = ?;

-- name: SetLuksOwnershipTaken :exec
INSERT INTO luks_state (action_id, device_path, ownership_taken, device_key_type, last_rotated_at)
VALUES (?, ?, TRUE, 'none', ?)
ON CONFLICT(action_id) DO UPDATE SET
    device_path = excluded.device_path,
    ownership_taken = TRUE,
    last_rotated_at = excluded.last_rotated_at;

-- name: SetLuksDeviceKeyType :exec
UPDATE luks_state SET device_key_type = ? WHERE action_id = ?;

-- name: SetLuksLastRotatedAt :exec
UPDATE luks_state SET last_rotated_at = ? WHERE action_id = ?;

-- name: DeleteLuksState :exec
DELETE FROM luks_state WHERE action_id = ?;

-- name: GetLuksPassphraseHashes :many
SELECT passphrase_hash
FROM luks_user_passphrase_history
WHERE action_id = ?
ORDER BY created_at DESC, id DESC
LIMIT 3;

-- name: AddLuksPassphraseHash :exec
INSERT INTO luks_user_passphrase_history (action_id, passphrase_hash) VALUES (?, ?);

-- name: PruneLuksPassphraseHashes :exec
DELETE FROM luks_user_passphrase_history AS history
WHERE history.action_id = ? AND history.id NOT IN (
    SELECT inner_history.id
    FROM luks_user_passphrase_history AS inner_history
    WHERE inner_history.action_id = ?
    ORDER BY created_at DESC, id DESC
    LIMIT 3
);

-- name: GetLpsState :many
SELECT action_id, username, last_rotated_at, password_hash
FROM lps_state
WHERE action_id = ?;

-- name: SetLpsUserState :exec
INSERT INTO lps_state (action_id, username, last_rotated_at, password_hash)
VALUES (?, ?, ?, ?)
ON CONFLICT(action_id, username) DO UPDATE SET
    last_rotated_at = excluded.last_rotated_at,
    password_hash = excluded.password_hash;

-- name: DeleteLpsState :exec
DELETE FROM lps_state WHERE action_id = ?;

-- name: GetSetting :one
SELECT value FROM settings WHERE key = ?;

-- name: SetSetting :exec
INSERT INTO settings (key, value) VALUES (?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value;

-- name: DeleteSetting :exec
DELETE FROM settings WHERE key = ?;
