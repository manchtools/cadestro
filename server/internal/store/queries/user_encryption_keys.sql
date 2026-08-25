




-- name: InsertUserEncryptionKey :execrows




INSERT INTO user_encryption_keys (user_id, wrapped_dek)
VALUES (?, ?)
ON CONFLICT (user_id) DO NOTHING;

-- name: GetUserEncryptionKey :one
SELECT * FROM user_encryption_keys WHERE user_id = ?;

-- name: DeleteUserEncryptionKey :execrows





DELETE FROM user_encryption_keys WHERE user_id = ?;

-- name: CountUserEncryptionKeys :one
SELECT COUNT(*) FROM user_encryption_keys;
