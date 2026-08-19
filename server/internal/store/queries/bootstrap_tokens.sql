-- The host-authorized bootstrap-admin token.
--
-- One row in `tokens`, distinguished by its reserved name. Only the
-- SHA-256 digest of the bearer value is stored: the token is printed
-- once by the command that mints it and is never recoverable. Owner is
-- NULL because the bootstrap principal is deliberately not a user.

-- name: InsertBootstrapAdminToken :one
INSERT INTO tokens (id, value_hash, name, max_uses, expires_at, created_at, created_by, disabled, is_deleted)
VALUES (sqlc.arg(id), sqlc.arg(value_hash), sqlc.arg(reserved_name), 1,
        sqlc.arg(expires_at), sqlc.arg(created_at), sqlc.arg(created_by), FALSE, FALSE)
RETURNING *;

-- name: ConsumeBootstrapAdminToken :one
-- The consume-once conditional write. Retiring the row is the bootstrap
-- boundary; enrollment tokens never mutate a usage counter.
UPDATE tokens
SET is_deleted = TRUE
WHERE value_hash = ?
  AND name = sqlc.arg(reserved_name)
  AND is_deleted = FALSE
  AND disabled = FALSE
  AND expires_at > sqlc.arg(now)
RETURNING *;

-- name: RetireBootstrapAdminTokens :execrows
-- Minting a new bootstrap token retires every outstanding one, so at
-- most one host-authorized token can ever be presentable.
UPDATE tokens SET is_deleted = TRUE
WHERE name = sqlc.arg(reserved_name) AND is_deleted = FALSE;

-- name: CountLiveBootstrapAdminTokens :one
SELECT COUNT(*) FROM tokens
WHERE name = sqlc.arg(reserved_name)
  AND is_deleted = FALSE
  AND disabled = FALSE
  AND expires_at > sqlc.arg(now);
