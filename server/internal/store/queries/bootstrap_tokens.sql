






-- name: InsertBootstrapAdminToken :one
INSERT INTO tokens (id, value_hash, name, max_uses, expires_at, created_at, created_by, disabled, is_deleted)
VALUES (sqlc.arg(id), sqlc.arg(value_hash), sqlc.arg(reserved_name), 1,
        sqlc.arg(expires_at), sqlc.arg(created_at), sqlc.arg(created_by), FALSE, FALSE)
RETURNING *;

-- name: ConsumeBootstrapAdminToken :one


UPDATE tokens
SET is_deleted = TRUE
WHERE value_hash = ?
  AND name = sqlc.arg(reserved_name)
  AND is_deleted = FALSE
  AND disabled = FALSE
  AND expires_at > sqlc.arg(now)
RETURNING *;

-- name: RetireBootstrapAdminTokens :execrows


UPDATE tokens SET is_deleted = TRUE
WHERE name = sqlc.arg(reserved_name) AND is_deleted = FALSE;

-- name: CountLiveBootstrapAdminTokens :one
SELECT COUNT(*) FROM tokens
WHERE name = sqlc.arg(reserved_name)
  AND is_deleted = FALSE
  AND disabled = FALSE
  AND expires_at > sqlc.arg(now);
