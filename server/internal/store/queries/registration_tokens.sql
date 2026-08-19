-- Registration-token CRUD. The reserved host bootstrap token is deliberately
-- absent from this surface: it has its own consume-once boundary.

-- name: GetRegistrationToken :one
SELECT * FROM tokens
WHERE id = sqlc.arg(id)
  AND is_deleted = FALSE
  AND name <> sqlc.arg(reserved_name);

-- name: ListRegistrationTokens :many
SELECT t.*, COUNT(d.id) AS current_uses
FROM tokens t
LEFT JOIN devices d ON d.registration_token_id = t.id
WHERE t.is_deleted = FALSE
  AND t.name <> sqlc.arg(reserved_name)
  AND t.id > sqlc.arg(after_id)
  AND (sqlc.arg(include_disabled) OR t.disabled = FALSE)
GROUP BY t.id
ORDER BY t.id
LIMIT sqlc.arg(row_limit);

-- name: CountRegistrationTokens :one
SELECT COUNT(*) FROM tokens
WHERE is_deleted = FALSE
  AND name <> sqlc.arg(reserved_name)
  AND (sqlc.arg(include_disabled) OR disabled = FALSE);

-- name: InsertRegistrationToken :one
INSERT INTO tokens (
    id, value_hash, name, max_uses,
    expires_at, created_at, created_by, disabled, is_deleted
) VALUES (
    sqlc.arg(id), sqlc.arg(value_hash), sqlc.arg(name), sqlc.arg(max_uses),
    sqlc.arg(expires_at), sqlc.arg(created_at), sqlc.arg(created_by), FALSE, FALSE
)
RETURNING *;

-- name: RenameRegistrationToken :one
UPDATE tokens
SET name = sqlc.arg(name)
WHERE id = sqlc.arg(id)
  AND is_deleted = FALSE
  AND name <> sqlc.arg(reserved_name)
RETURNING *;

-- name: SetRegistrationTokenDisabled :one
UPDATE tokens
SET disabled = sqlc.arg(disabled)
WHERE id = sqlc.arg(id)
  AND is_deleted = FALSE
  AND name <> sqlc.arg(reserved_name)
RETURNING *;

-- name: SoftDeleteRegistrationToken :one
UPDATE tokens
SET is_deleted = TRUE
WHERE id = sqlc.arg(id)
  AND is_deleted = FALSE
  AND name <> sqlc.arg(reserved_name)
RETURNING *;

-- Usage is immutable device provenance, including soft-deleted devices.
-- name: CountRegistrationTokenUses :one
SELECT COUNT(*) FROM devices WHERE registration_token_id = sqlc.arg(token_id);
