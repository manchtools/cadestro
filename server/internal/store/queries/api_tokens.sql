-- name: InsertApiToken :one
INSERT INTO api_tokens (id, user_id, name, expires_at, created_at)
VALUES (sqlc.arg(id), sqlc.arg(user_id), sqlc.arg(name), sqlc.arg(expires_at), sqlc.arg(created_at))
RETURNING *;

-- name: ListApiTokensForUser :many
SELECT * FROM api_tokens
WHERE user_id = sqlc.arg(user_id)
  AND id > sqlc.arg(after_id)
ORDER BY id
LIMIT sqlc.arg(row_limit);

-- name: CountApiTokensForUser :one
SELECT COUNT(*) FROM api_tokens WHERE user_id = sqlc.arg(user_id);

-- name: GetApiTokenForAuth :one
SELECT * FROM api_tokens
WHERE id = sqlc.arg(id)
  AND user_id = sqlc.arg(user_id)
  AND revoked_at IS NULL;

-- name: RevokeApiToken :one
UPDATE api_tokens
SET revoked_at = sqlc.arg(revoked_at)
WHERE id = sqlc.arg(id)
  AND user_id = sqlc.arg(user_id)
  AND revoked_at IS NULL
RETURNING *;
