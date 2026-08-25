






-- name: InsertUserRoleGrant :one
INSERT INTO user_roles (grant_id, user_id, role_id, assigned_at, assigned_by, scope_kind, scope_id)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: DeleteUnscopedUserRoleGrant :one



DELETE FROM user_roles
WHERE user_id = ? AND role_id = ? AND scope_id IS NULL
RETURNING *;

-- name: DeleteScopedUserRoleGrant :one
DELETE FROM user_roles
WHERE user_id = ? AND role_id = ? AND scope_kind = ? AND scope_id = ?
RETURNING *;

-- name: DeleteUserRoleGrantsForUser :execrows
DELETE FROM user_roles WHERE user_id = ?;

-- name: ListUserRoleGrants :many
SELECT
    ur.grant_id,
    ur.scope_kind,
    ur.scope_id,
    sqlc.embed(r)
FROM user_roles ur
JOIN roles r ON r.id = ur.role_id
WHERE ur.user_id = sqlc.arg(user_id) AND r.is_deleted = FALSE
ORDER BY ur.grant_id;

-- name: InsertUserGroupRoleGrant :one
INSERT INTO user_group_roles (grant_id, group_id, role_id, assigned_at, assigned_by, scope_kind, scope_id)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: DeleteUnscopedUserGroupRoleGrant :one
DELETE FROM user_group_roles
WHERE group_id = ? AND role_id = ? AND scope_id IS NULL
RETURNING *;

-- name: DeleteScopedUserGroupRoleGrant :one
DELETE FROM user_group_roles
WHERE group_id = ? AND role_id = ? AND scope_kind = ? AND scope_id = ?
RETURNING *;

-- name: ListUserGroupRoleGrants :many
SELECT
    gr.grant_id,
    gr.scope_kind,
    gr.scope_id,
    sqlc.embed(r)
FROM user_group_roles gr
JOIN roles r ON r.id = gr.role_id
WHERE gr.group_id = ? AND r.is_deleted = FALSE
ORDER BY gr.grant_id;

-- name: ListInheritedRolesForUser :many



SELECT
    r.id   AS role_id,
    r.name AS role_name,
    g.id   AS group_id,
    g.name AS group_name
FROM user_group_members m
JOIN user_groups g ON g.id = m.group_id AND g.is_deleted = FALSE
JOIN user_group_roles gr ON gr.group_id = m.group_id
JOIN roles r ON r.id = gr.role_id AND r.is_deleted = FALSE
WHERE m.user_id = sqlc.arg(user_id)
ORDER BY r.id, g.id;

-- name: ListUserPermissions :many


SELECT DISTINCT s.permission AS permission
FROM (
    SELECT CAST(permission.value AS TEXT) AS permission
      FROM user_roles ur
      JOIN roles r ON r.id = ur.role_id AND r.is_deleted = FALSE
      JOIN json_each(r.permissions) permission
     WHERE ur.user_id = sqlc.arg(user_id)
    UNION ALL
    SELECT CAST(permission.value AS TEXT) AS permission
      FROM user_group_members m
	  JOIN user_groups g ON g.id = m.group_id AND g.is_deleted = FALSE
      JOIN user_group_roles gr ON gr.group_id = m.group_id
      JOIN roles r ON r.id = gr.role_id AND r.is_deleted = FALSE
      JOIN json_each(r.permissions) permission
     WHERE m.user_id = sqlc.arg(user_id)
) s
ORDER BY permission;

-- name: ListUserScopedGrants :many



SELECT DISTINCT s.permission AS permission, s.scope_kind, s.scope_id
FROM (
    SELECT CAST(permission.value AS TEXT) AS permission, ur.scope_kind, ur.scope_id
      FROM user_roles ur
      JOIN roles r ON r.id = ur.role_id AND r.is_deleted = FALSE
      JOIN json_each(r.permissions) permission
     WHERE ur.user_id = sqlc.arg(user_id)
    UNION ALL
    SELECT CAST(permission.value AS TEXT) AS permission, gr.scope_kind, gr.scope_id
      FROM user_group_members m
	  JOIN user_groups g ON g.id = m.group_id AND g.is_deleted = FALSE
      JOIN user_group_roles gr ON gr.group_id = m.group_id
      JOIN roles r ON r.id = gr.role_id AND r.is_deleted = FALSE
      JOIN json_each(r.permissions) permission
     WHERE m.user_id = sqlc.arg(user_id)
) s
ORDER BY permission, s.scope_kind NULLS FIRST, s.scope_id;

-- name: ListUserGroupIDsForUser :many
SELECT m.group_id
FROM user_group_members m
JOIN user_groups g ON g.id = m.group_id AND g.is_deleted = FALSE
WHERE m.user_id = sqlc.arg(user_id)
ORDER BY m.group_id;

-- name: ListRoleHolderIDs :many
SELECT user_id FROM (
    SELECT ur.user_id
    FROM user_roles ur
    WHERE ur.role_id = sqlc.arg(role_id)
    UNION
    SELECT m.user_id
    FROM user_group_roles gr
    JOIN user_groups g ON g.id = gr.group_id AND g.is_deleted = FALSE
    JOIN user_group_members m ON m.group_id = gr.group_id
    WHERE gr.role_id = sqlc.arg(role_id)
)
ORDER BY user_id;

-- name: DeleteUserGroupMembershipsForUser :execrows
DELETE FROM user_group_members WHERE user_id = ?;

-- name: BumpSessionVersionForUserGroupMembers :execrows
UPDATE users
SET session_version = session_version + 1, updated_at = sqlc.arg(updated_at)
WHERE is_deleted = FALSE
  AND id IN (SELECT user_id FROM user_group_members WHERE group_id = sqlc.arg(group_id));

-- name: CountEnabledUnscopedAdmins :one
SELECT COUNT(DISTINCT u.id)
FROM users u
WHERE u.is_deleted = FALSE
  AND u.disabled = FALSE
  AND (
      EXISTS (
          SELECT 1
          FROM user_roles ur
          JOIN roles r ON r.id = ur.role_id AND r.is_deleted = FALSE
          WHERE ur.user_id = u.id
            AND ur.scope_kind IS NULL
            AND ur.scope_id IS NULL
            AND r.name = 'Admin'
      )
      OR EXISTS (
          SELECT 1
          FROM user_group_members m
          JOIN user_groups g ON g.id = m.group_id AND g.is_deleted = FALSE
          JOIN user_group_roles gr ON gr.group_id = m.group_id
          JOIN roles r ON r.id = gr.role_id AND r.is_deleted = FALSE
          WHERE m.user_id = u.id
            AND gr.scope_kind IS NULL
            AND gr.scope_id IS NULL
            AND r.name = 'Admin'
      )
  );
