




-- name: InsertRole :one
INSERT INTO roles (id, name, description, permissions, is_system, created_at, created_by, updated_at)
VALUES (?, ?, ?, ?, FALSE, ?, ?, ?)
RETURNING *;

-- name: GetRole :one
SELECT * FROM roles WHERE id = ? AND is_deleted = FALSE;

-- name: GetRoleByName :one
SELECT * FROM roles WHERE name = ? AND is_deleted = FALSE;

-- name: ListRoles :many



SELECT * FROM roles
WHERE is_deleted = FALSE AND id > ?
ORDER BY id
LIMIT ?;

-- name: CountRoles :one
SELECT COUNT(*) FROM roles WHERE is_deleted = FALSE;

-- name: UpdateRole :one



UPDATE roles
SET name = ?, description = ?, permissions = ?, updated_at = ?
WHERE id = ? AND is_deleted = FALSE AND is_system = FALSE
RETURNING *;

-- name: SoftDeleteRole :execrows
UPDATE roles SET is_deleted = TRUE, updated_at = ?
WHERE id = ? AND is_deleted = FALSE AND is_system = FALSE;

-- name: UpdateSystemRolePermissions :execrows


UPDATE roles SET permissions = ?, updated_at = ?
WHERE id = ? AND is_system = TRUE AND is_deleted = FALSE;

-- name: CountRoleHolders :one

SELECT COUNT(*) FROM (
    SELECT ur.user_id FROM user_roles ur WHERE ur.role_id = sqlc.arg(role_id)
    UNION
    SELECT m.user_id
      FROM user_group_roles gr
      JOIN user_group_members m ON m.group_id = gr.group_id
     WHERE gr.role_id = sqlc.arg(role_id)
) holders;

-- name: BumpSessionVersionForRoleHolders :execrows




UPDATE users SET session_version = session_version + 1, updated_at = ?
WHERE is_deleted = FALSE AND id IN (
    SELECT ur.user_id FROM user_roles ur WHERE ur.role_id = sqlc.arg(role_id)
    UNION
    SELECT m.user_id
      FROM user_group_roles gr
      JOIN user_group_members m ON m.group_id = gr.group_id
     WHERE gr.role_id = sqlc.arg(role_id)
);
