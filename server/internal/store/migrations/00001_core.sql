-- +goose Up

CREATE TABLE identity_providers (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    client_id TEXT NOT NULL,
    issuer_url TEXT NOT NULL,
    scopes_json TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);

CREATE TABLE users (
    id TEXT PRIMARY KEY,
    email TEXT NOT NULL,
    display_name TEXT NOT NULL DEFAULT '',
    session_version INTEGER NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    last_login_at DATETIME NOT NULL
);

CREATE TABLE roles (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);

CREATE TABLE role_permissions (
    role_id TEXT NOT NULL,
    permission INTEGER NOT NULL CHECK (permission BETWEEN 1 AND 46),
    PRIMARY KEY (role_id, permission),
    FOREIGN KEY (role_id) REFERENCES roles(id) ON DELETE CASCADE
);

INSERT INTO roles (id, name, description, created_at, updated_at)
VALUES
    ('01J00000000000000000000001', 'Administrators', 'Full control-plane access', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    ('01J00000000000000000000002', 'Users', 'Read current user', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);

INSERT INTO role_permissions (role_id, permission) VALUES
    ('01J00000000000000000000001', 1), ('01J00000000000000000000001', 2),
    ('01J00000000000000000000001', 3), ('01J00000000000000000000001', 4),
    ('01J00000000000000000000001', 5), ('01J00000000000000000000001', 6),
    ('01J00000000000000000000001', 7), ('01J00000000000000000000001', 8),
    ('01J00000000000000000000001', 9), ('01J00000000000000000000001', 10),
    ('01J00000000000000000000001', 11), ('01J00000000000000000000001', 12),
    ('01J00000000000000000000001', 13), ('01J00000000000000000000001', 14),
    ('01J00000000000000000000001', 15), ('01J00000000000000000000001', 16),
    ('01J00000000000000000000001', 17), ('01J00000000000000000000001', 18),
    ('01J00000000000000000000001', 19), ('01J00000000000000000000001', 20),
    ('01J00000000000000000000001', 21), ('01J00000000000000000000001', 22),
    ('01J00000000000000000000001', 23), ('01J00000000000000000000001', 24),
    ('01J00000000000000000000001', 25), ('01J00000000000000000000001', 26),
    ('01J00000000000000000000001', 27), ('01J00000000000000000000001', 28),
    ('01J00000000000000000000001', 29), ('01J00000000000000000000001', 30),
    ('01J00000000000000000000001', 31), ('01J00000000000000000000001', 32),
    ('01J00000000000000000000001', 33), ('01J00000000000000000000001', 34),
    ('01J00000000000000000000001', 35), ('01J00000000000000000000001', 36),
    ('01J00000000000000000000001', 37), ('01J00000000000000000000001', 38),
    ('01J00000000000000000000001', 39), ('01J00000000000000000000001', 40),
    ('01J00000000000000000000001', 41), ('01J00000000000000000000001', 42),
    ('01J00000000000000000000001', 43), ('01J00000000000000000000001', 44),
    ('01J00000000000000000000001', 45), ('01J00000000000000000000001', 46);

INSERT INTO role_permissions (role_id, permission) VALUES
    ('01J00000000000000000000002', 1);

CREATE TABLE user_roles (
    user_id TEXT NOT NULL,
    role_id TEXT NOT NULL,
    PRIMARY KEY (user_id, role_id),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (role_id) REFERENCES roles(id) ON DELETE CASCADE
);

CREATE TABLE identity_links (
    provider_id TEXT NOT NULL,
    subject TEXT NOT NULL,
    user_id TEXT NOT NULL,
    PRIMARY KEY (provider_id, subject),
    FOREIGN KEY (provider_id) REFERENCES identity_providers(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE registration_tokens (
    id TEXT PRIMARY KEY,
    value_hash TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    max_uses INTEGER NOT NULL DEFAULT 0,
    current_uses INTEGER NOT NULL DEFAULT 0,
    expires_at DATETIME NOT NULL,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);

CREATE TABLE devices (
    id TEXT PRIMARY KEY,
    hostname TEXT NOT NULL,
    agent_version TEXT NOT NULL,
    identity_public_key BLOB NOT NULL UNIQUE,
    active_certificate_pem BLOB NOT NULL,
    active_cert_serial TEXT NOT NULL,
    cert_expires_at DATETIME NOT NULL,
    pending_certificate_pem BLOB,
    pending_cert_serial TEXT,
    pending_cert_expires_at DATETIME,
    registered_at DATETIME NOT NULL,
    last_seen_at DATETIME,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);

CREATE TABLE actions (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    action_blob BLOB NOT NULL,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);

CREATE TABLE device_groups (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);

CREATE TABLE device_group_members (
    group_id TEXT NOT NULL,
    device_id TEXT NOT NULL,
    PRIMARY KEY (group_id, device_id),
    FOREIGN KEY (group_id) REFERENCES device_groups(id) ON DELETE CASCADE,
    FOREIGN KEY (device_id) REFERENCES devices(id) ON DELETE CASCADE
);

CREATE TABLE assignments (
    id TEXT PRIMARY KEY,
    action_id TEXT NOT NULL,
    target_type INTEGER NOT NULL,
    target_id TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    UNIQUE (action_id, target_type, target_id),
    FOREIGN KEY (action_id) REFERENCES actions(id) ON DELETE CASCADE
);

CREATE TABLE execution_results (
    run_id TEXT PRIMARY KEY,
    device_id TEXT NOT NULL,
    action_id TEXT NOT NULL,
    completed_at DATETIME NOT NULL,
    result_blob BLOB NOT NULL,
    FOREIGN KEY (device_id) REFERENCES devices(id) ON DELETE CASCADE,
    FOREIGN KEY (action_id) REFERENCES actions(id) ON DELETE CASCADE
);

CREATE INDEX execution_results_device_completed ON execution_results(device_id, completed_at DESC, run_id DESC);
CREATE INDEX execution_results_device_action_completed ON execution_results(device_id, action_id, completed_at DESC, run_id DESC);

CREATE TABLE audit_events (
    id TEXT PRIMARY KEY,
    event_type INTEGER NOT NULL CHECK (event_type BETWEEN 1 AND 19),
    stream_type INTEGER NOT NULL CHECK (stream_type BETWEEN 1 AND 7),
    stream_id TEXT NOT NULL,
    actor_type INTEGER NOT NULL CHECK (actor_type BETWEEN 1 AND 3),
    actor_id TEXT NOT NULL,
    occurred_at DATETIME NOT NULL
);

CREATE INDEX audit_events_time ON audit_events(occurred_at DESC, id DESC);

-- +goose Down

DROP TABLE audit_events;
DROP TABLE execution_results;
DROP TABLE assignments;
DROP TABLE device_group_members;
DROP TABLE device_groups;
DROP TABLE actions;
DROP TABLE devices;
DROP TABLE registration_tokens;
DROP TABLE user_roles;
DROP TABLE role_permissions;
DROP TABLE roles;
DROP TABLE identity_links;
DROP TABLE users;
DROP TABLE identity_providers;
