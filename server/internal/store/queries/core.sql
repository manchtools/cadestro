-- name: CountIdentityProviders :one
SELECT COUNT(*) FROM identity_providers;

-- name: CreateIdentityProvider :one
INSERT INTO identity_providers (id, name, slug, enabled, client_id, issuer_url, scopes_json, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
RETURNING *;

-- name: GetIdentityProvider :one
SELECT * FROM identity_providers WHERE id = ?;

-- name: GetIdentityProviderBySlug :one
SELECT * FROM identity_providers WHERE slug = ?;

-- name: ListIdentityProviders :many
SELECT * FROM identity_providers ORDER BY name, id;

-- name: ListEnabledIdentityProviders :many
SELECT * FROM identity_providers WHERE enabled = TRUE ORDER BY name, id;

-- name: ConfigureIdentityProvider :one
UPDATE identity_providers
SET client_id = ?, issuer_url = ?, scopes_json = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?
RETURNING *;

-- name: RenameIdentityProvider :one
UPDATE identity_providers SET name = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? RETURNING *;

-- name: EnableIdentityProvider :one
UPDATE identity_providers SET enabled = TRUE, updated_at = CURRENT_TIMESTAMP WHERE id = ? RETURNING *;

-- name: DisableIdentityProvider :one
UPDATE identity_providers SET enabled = FALSE, updated_at = CURRENT_TIMESTAMP WHERE id = ? RETURNING *;

-- name: DeleteIdentityProvider :execrows
DELETE FROM identity_providers WHERE id = ?;

-- name: GetUser :one
SELECT * FROM users WHERE id = ?;

-- name: CreateUser :one
INSERT INTO users (id, email, display_name, session_version, created_at, updated_at, last_login_at)
VALUES (?, ?, ?, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, ?)
RETURNING *;

-- name: LinkIdentity :exec
INSERT INTO identity_links (provider_id, subject, user_id) VALUES (?, ?, ?);

-- name: UpdateIdentityUserLogin :one
UPDATE users
SET email = ?, display_name = ?, session_version = session_version + 1, last_login_at = ?, updated_at = CURRENT_TIMESTAMP
WHERE users.id = (
    SELECT identity_links.user_id FROM identity_links
    WHERE identity_links.provider_id = ? AND identity_links.subject = ?
)
RETURNING *;

-- name: RotateUserSession :one
UPDATE users
SET session_version = session_version + 1, updated_at = CURRENT_TIMESTAMP
WHERE id = ? AND session_version = ?
RETURNING *;

-- name: CreateRole :one
INSERT INTO roles (id, name, description, created_at, updated_at)
VALUES (?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
RETURNING *;

-- name: GetRole :one
SELECT * FROM roles WHERE id = ?;

-- name: GetRoleByName :one
SELECT * FROM roles WHERE name = ?;

-- name: ListRoles :many
SELECT * FROM roles WHERE id > ? ORDER BY id LIMIT ?;

-- name: RenameRole :one
UPDATE roles SET name = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? RETURNING *;

-- name: SetRoleDescription :one
UPDATE roles SET description = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? RETURNING *;

-- name: DeleteRole :execrows
DELETE FROM roles WHERE id = ?;

-- name: GrantRolePermission :one
INSERT INTO role_permissions (role_id, permission)
SELECT roles.id, sqlc.arg(permission) FROM roles
WHERE roles.id = sqlc.arg(role_id)
RETURNING role_id;

-- name: RevokeRolePermission :execrows
DELETE FROM role_permissions WHERE role_id = ? AND permission = ?;

-- name: TouchRole :one
UPDATE roles SET updated_at = CURRENT_TIMESTAMP WHERE id = ? RETURNING *;

-- name: ListRolePermissions :many
SELECT permission FROM role_permissions WHERE role_id = ? ORDER BY permission;

-- name: ListUserRoles :many
SELECT roles.* FROM user_roles JOIN roles ON roles.id = user_roles.role_id
WHERE user_roles.user_id = ? ORDER BY roles.id;

-- name: ListUserPermissions :many
SELECT DISTINCT role_permissions.permission
FROM user_roles JOIN role_permissions ON role_permissions.role_id = user_roles.role_id
WHERE user_roles.user_id = ? ORDER BY role_permissions.permission;

-- name: AssignInitialRole :exec
INSERT INTO user_roles (user_id, role_id)
SELECT sqlc.arg(user_id), roles.id AS role_id
FROM roles
WHERE roles.id = (
    SELECT CASE WHEN COUNT(*) = 1 THEN sqlc.arg(administrators_role_id) ELSE sqlc.arg(users_role_id) END AS role_id FROM users
);

-- name: AssignRoleToUser :one
INSERT INTO user_roles (user_id, role_id)
SELECT users.id, roles.id
FROM users JOIN roles
WHERE users.id = sqlc.arg(user_id) AND roles.id = sqlc.arg(role_id)
RETURNING user_id;

-- name: RevokeRoleFromUser :execrows
DELETE FROM user_roles WHERE user_id = ? AND role_id = ?;

-- name: BumpSessionsForRole :exec
UPDATE users SET session_version = session_version + 1, updated_at = CURRENT_TIMESTAMP
WHERE id IN (SELECT user_id FROM user_roles WHERE role_id = ?);

-- name: RotateUserSessionByID :one
UPDATE users SET session_version = session_version + 1, updated_at = CURRENT_TIMESTAMP WHERE id = ? RETURNING *;

-- name: ListUsers :many
SELECT * FROM users WHERE id > ? ORDER BY id LIMIT ?;

-- name: CreateRegistrationToken :one
INSERT INTO registration_tokens (id, value_hash, name, max_uses, current_uses, expires_at, created_at, updated_at)
VALUES (?, ?, ?, ?, 0, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
RETURNING *;

-- name: GetRegistrationToken :one
SELECT * FROM registration_tokens WHERE id = ?;

-- name: GetUsableRegistrationToken :one
SELECT * FROM registration_tokens
WHERE value_hash = ? AND expires_at > ? AND (max_uses = 0 OR current_uses < max_uses);

-- name: ConsumeRegistrationToken :one
UPDATE registration_tokens SET current_uses = current_uses + 1, updated_at = CURRENT_TIMESTAMP
WHERE id = ? AND expires_at > ? AND (max_uses = 0 OR current_uses < max_uses)
RETURNING *;

-- name: ConsumeFinalRegistrationToken :one
DELETE FROM registration_tokens
WHERE id = ? AND expires_at > ?
  AND (max_uses = 0 OR current_uses < max_uses)
  AND max_uses > 0 AND current_uses + 1 >= max_uses
RETURNING *;

-- name: ListRegistrationTokens :many
SELECT * FROM registration_tokens
WHERE id > sqlc.arg(after_id)
ORDER BY id LIMIT sqlc.arg(page_limit);

-- name: CountRegistrationTokens :one
SELECT COUNT(*) FROM registration_tokens;

-- name: RenameRegistrationToken :one
UPDATE registration_tokens SET name = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? RETURNING *;

-- name: DeleteRegistrationToken :execrows
DELETE FROM registration_tokens WHERE id = ?;

-- name: CreateDevice :one
INSERT INTO devices (id, hostname, agent_version, identity_public_key, active_certificate_pem, active_cert_serial, cert_expires_at, registered_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
RETURNING *;

-- name: GetDevice :one
SELECT * FROM devices WHERE id = ?;

-- name: FindDeviceByIdentityKey :one
SELECT * FROM devices WHERE identity_public_key = ?;

-- name: SetPendingDeviceCertificate :execrows
UPDATE devices
SET pending_certificate_pem = ?, pending_cert_serial = ?, pending_cert_expires_at = ?,
    updated_at = CURRENT_TIMESTAMP
WHERE id = ? AND active_cert_serial = ?;

-- name: PromotePendingDeviceCertificate :execrows
UPDATE devices
SET active_certificate_pem = pending_certificate_pem,
    active_cert_serial = pending_cert_serial,
    cert_expires_at = pending_cert_expires_at,
    pending_certificate_pem = NULL,
    pending_cert_serial = NULL,
    pending_cert_expires_at = NULL,
    updated_at = CURRENT_TIMESTAMP
WHERE id = ? AND pending_cert_serial = ?;

-- name: TouchDevice :exec
UPDATE devices SET hostname = ?, agent_version = ?, last_seen_at = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?;

-- name: ListDevices :many
SELECT * FROM devices WHERE id > ? ORDER BY id LIMIT ?;

-- name: CountDevices :one
SELECT COUNT(*) FROM devices;

-- name: DeleteDevice :execrows
DELETE FROM devices WHERE id = ?;

-- name: CreateAction :one
INSERT INTO actions (id, name, description, action_blob, created_at, updated_at)
VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
RETURNING *;

-- name: GetAction :one
SELECT * FROM actions WHERE id = ?;

-- name: ListActions :many
SELECT * FROM actions
WHERE id > sqlc.arg(after_id)
ORDER BY id LIMIT sqlc.arg(page_limit);

-- name: CountActions :one
SELECT COUNT(*) FROM actions;

-- name: RenameAction :one
UPDATE actions SET name = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? RETURNING *;

-- name: SetActionDescription :one
UPDATE actions SET description = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? RETURNING *;

-- name: ConfigureAction :one
UPDATE actions SET action_blob = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ? RETURNING *;

-- name: DeleteAction :execrows
DELETE FROM actions WHERE id = ?;

-- name: CreateDeviceGroup :one
INSERT INTO device_groups (id, name, description, created_at, updated_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP) RETURNING *;

-- name: GetDeviceGroup :one
SELECT device_groups.*, COUNT(device_group_members.device_id) AS member_count
FROM device_groups LEFT JOIN device_group_members ON device_group_members.group_id = device_groups.id
WHERE device_groups.id = ? GROUP BY device_groups.id;

-- name: ListDeviceGroups :many
SELECT device_groups.*, COUNT(device_group_members.device_id) AS member_count
FROM device_groups LEFT JOIN device_group_members ON device_group_members.group_id = device_groups.id
WHERE device_groups.id > ? GROUP BY device_groups.id ORDER BY device_groups.id LIMIT ?;

-- name: CountDeviceGroups :one
SELECT COUNT(*) FROM device_groups;

-- name: ListDeviceGroupsForDevice :many
SELECT device_groups.*, COUNT(all_members.device_id) AS member_count
FROM device_groups
JOIN device_group_members selected ON selected.group_id = device_groups.id AND selected.device_id = ?
LEFT JOIN device_group_members all_members ON all_members.group_id = device_groups.id
GROUP BY device_groups.id ORDER BY device_groups.name, device_groups.id;

-- name: ListDeviceGroupMembers :many
SELECT devices.* FROM device_group_members JOIN devices ON devices.id = device_group_members.device_id
WHERE device_group_members.group_id = ? ORDER BY devices.hostname, devices.id;

-- name: RenameDeviceGroup :one
UPDATE device_groups SET name = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? RETURNING *;

-- name: SetDeviceGroupDescription :one
UPDATE device_groups SET description = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? RETURNING *;

-- name: DeleteDeviceGroup :execrows
DELETE FROM device_groups WHERE id = ?;

-- name: AddDeviceToGroup :exec
INSERT INTO device_group_members (group_id, device_id) VALUES (?, ?);

-- name: RemoveDeviceFromGroup :execrows
DELETE FROM device_group_members WHERE group_id = ? AND device_id = ?;

-- name: CreateAssignment :one
INSERT INTO assignments (id, action_id, target_type, target_id, created_at) VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP) RETURNING *;

-- name: GetAssignment :one
SELECT * FROM assignments WHERE id = ?;

-- name: DeleteAssignment :execrows
DELETE FROM assignments WHERE id = ?;

-- name: ListAssignments :many
SELECT assignments.*, actions.name AS action_name,
CAST(COALESCE(CASE assignments.target_type WHEN 1 THEN devices.hostname ELSE device_groups.name END, '') AS TEXT) AS target_name
FROM assignments JOIN actions ON actions.id = assignments.action_id
LEFT JOIN devices ON assignments.target_type = 1 AND devices.id = assignments.target_id
LEFT JOIN device_groups ON assignments.target_type = 2 AND device_groups.id = assignments.target_id
WHERE (CAST(sqlc.arg(action_filter) AS TEXT) = '' OR assignments.action_id = CAST(sqlc.arg(action_filter) AS TEXT))
  AND (CAST(sqlc.arg(target_type_filter) AS INTEGER) = 0 OR assignments.target_type = CAST(sqlc.arg(target_type_filter) AS INTEGER))
  AND (CAST(sqlc.arg(target_filter) AS TEXT) = '' OR assignments.target_id = CAST(sqlc.arg(target_filter) AS TEXT))
ORDER BY assignments.created_at, assignments.id;

-- name: ListActionsForDevice :many
SELECT DISTINCT actions.* FROM actions
JOIN assignments ON assignments.action_id = actions.id
LEFT JOIN device_group_members ON assignments.target_type = 2
    AND assignments.target_id = device_group_members.group_id
    AND device_group_members.device_id = ?
WHERE (assignments.target_type = 1 AND assignments.target_id = ?)
   OR (assignments.target_type = 2 AND device_group_members.device_id IS NOT NULL)
ORDER BY actions.id;

-- name: CreateExecutionResult :exec
INSERT INTO execution_results (
    run_id, device_id, action_id, completed_at, result_blob
) VALUES (?, ?, ?, ?, ?)
ON CONFLICT(run_id) DO NOTHING;

-- name: ListExecutionResults :many
SELECT execution_results.*, actions.name AS action_name, actions.action_blob FROM execution_results
JOIN actions ON actions.id = execution_results.action_id
WHERE execution_results.device_id = ? ORDER BY execution_results.completed_at DESC LIMIT ?;

-- name: ListComplianceResults :many
SELECT execution_results.*, actions.name AS action_name, actions.action_blob FROM execution_results
JOIN actions ON actions.id = execution_results.action_id
WHERE execution_results.device_id = ? AND execution_results.completed_at = (
      SELECT MAX(latest.completed_at) FROM execution_results latest
      WHERE latest.device_id = execution_results.device_id AND latest.action_id = execution_results.action_id
  )
ORDER BY actions.name, actions.id;

-- name: CreateAuditEvent :exec
INSERT INTO audit_events (id, event_type, stream_type, stream_id, actor_type, actor_id, occurred_at)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: ListAuditEvents :many
SELECT * FROM audit_events WHERE id < ? ORDER BY id DESC LIMIT ?;
