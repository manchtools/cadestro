



-- name: InsertIdentityProvider :one
INSERT INTO identity_providers (
    id, name, slug, provider_type, enabled,
    client_id, client_secret_encrypted,
    issuer_url, authorization_url, token_url, userinfo_url,
    scopes, auto_create_users, auto_link_by_email, trust_email_assertions,
    default_role_id, group_claim, group_mapping,
    created_at, created_by, updated_at
)
VALUES (
    ?, ?, ?, ?, ?,
    ?, ?,
    ?, ?, ?, ?,
    ?, ?, ?, ?,
    ?, ?, ?,
    ?, ?, ?
)
RETURNING *;

-- name: GetIdentityProvider :one
SELECT * FROM identity_providers WHERE id = ? AND is_deleted = FALSE;

-- name: GetIdentityProviderBySlug :one
SELECT * FROM identity_providers WHERE slug = ? AND is_deleted = FALSE;

-- name: ListIdentityProviders :many
SELECT * FROM identity_providers
WHERE is_deleted = FALSE AND id > ?
ORDER BY id
LIMIT ?;

-- name: ListEnabledIdentityProviders :many
SELECT * FROM identity_providers
WHERE is_deleted = FALSE AND enabled = TRUE
ORDER BY name;

-- name: CountIdentityProviders :one
SELECT COUNT(*) FROM identity_providers WHERE is_deleted = FALSE;

-- name: UpdateIdentityProvider :one
UPDATE identity_providers
SET name = ?,
    enabled = ?,
    client_id = ?,
    client_secret_encrypted = ?,
    issuer_url = ?,
    authorization_url = ?,
    token_url = ?,
    userinfo_url = ?,
    scopes = ?,
    auto_create_users = ?,
    auto_link_by_email = ?,
    trust_email_assertions = ?,
    default_role_id = ?,
    group_claim = ?,
    group_mapping = ?,
    updated_at = ?
WHERE id = ? AND is_deleted = FALSE
RETURNING *;

-- name: SoftDeleteIdentityProvider :execrows


UPDATE identity_providers SET is_deleted = TRUE, updated_at = ?
WHERE id = ? AND is_deleted = FALSE;

-- name: SetIdentityProviderSCIM :one



UPDATE identity_providers
SET scim_enabled = ?, scim_token_hash = ?, updated_at = ?
WHERE id = ? AND is_deleted = FALSE
RETURNING *;
