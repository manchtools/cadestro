-- name: InsertUser :one


INSERT INTO users (
    id, email, display_name, given_name, family_name, preferred_username,
    linux_username, linux_uid, provisioning_source, created_at, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetUser :one
SELECT * FROM users WHERE id = ? AND is_deleted = FALSE;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = ? AND is_deleted = FALSE;

-- name: GetUserSessionState :one


SELECT id, disabled, is_deleted, session_version FROM users WHERE id = ?;

-- name: ListUsers :many



SELECT * FROM users
WHERE is_deleted = FALSE AND id > ?
ORDER BY id
LIMIT ?;

-- name: UpdateUserEmail :execrows
UPDATE users SET email = ?, updated_at = ? WHERE id = ? AND is_deleted = FALSE;

-- name: UpdateUserProfile :one
UPDATE users
SET display_name = ?,
    given_name = ?,
    family_name = ?,
    preferred_username = ?,
    picture = ?,
    locale = ?,
    updated_at = ?
WHERE id = ? AND is_deleted = FALSE
RETURNING *;

-- name: UpdateUserSshSettings :one
UPDATE users
SET ssh_access_enabled = ?, ssh_allow_pubkey = ?, ssh_allow_password = ?, updated_at = ?
WHERE id = ? AND is_deleted = FALSE
RETURNING *;

-- name: UpdateUserLinuxUsername :one
UPDATE users SET linux_username = ?, updated_at = ?
WHERE id = ? AND is_deleted = FALSE
RETURNING *;

-- name: SetUserProvisioningEnabled :one
UPDATE users SET user_provisioning_enabled = ?, updated_at = ?
WHERE id = ? AND is_deleted = FALSE
RETURNING *;

-- name: SetUserDisabled :execrows


UPDATE users
SET disabled = sqlc.arg(disabled), session_version = session_version + 1, updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id) AND is_deleted = FALSE AND disabled <> sqlc.arg(disabled);

-- name: BumpUserSessionVersion :one



UPDATE users SET session_version = session_version + 1, updated_at = ?
WHERE id = ? AND is_deleted = FALSE
RETURNING session_version;

-- name: TouchUserLastLogin :execrows
UPDATE users SET last_login_at = ?, updated_at = ? WHERE id = ? AND is_deleted = FALSE;

-- name: DeleteUser :execrows


DELETE FROM users WHERE id = ?;

-- name: CountUsers :one
SELECT COUNT(*) FROM users WHERE is_deleted = FALSE;

-- name: GetNextLinuxUID :one
UPDATE linux_uid_sequence
SET next_value = next_value + 1
WHERE id = 1
RETURNING next_value - 1;

-- name: GetServerSettings :one
SELECT * FROM server_settings WHERE id = '00000000000000000000000003';

-- name: UpdateServerSettings :one
UPDATE server_settings
SET user_provisioning_enabled = sqlc.arg(user_provisioning_enabled),
    ssh_access_for_all = sqlc.arg(ssh_access_for_all),
    updated_at = sqlc.arg(updated_at)
WHERE id = '00000000000000000000000003'
RETURNING *;
