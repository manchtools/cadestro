-- +goose Up

CREATE TABLE identity_providers (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    client_id TEXT NOT NULL,
    client_secret TEXT NOT NULL,
    issuer_url TEXT NOT NULL,
    scopes_json TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);

CREATE TABLE users (
    id TEXT PRIMARY KEY,
    email TEXT NOT NULL,
    display_name TEXT NOT NULL DEFAULT '',
    picture TEXT NOT NULL DEFAULT '',
    session_version INTEGER NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL,
    last_login_at DATETIME NOT NULL
);

CREATE TABLE identity_links (
    provider_id TEXT NOT NULL,
    subject TEXT NOT NULL,
    user_id TEXT NOT NULL,
    PRIMARY KEY (provider_id, subject),
    FOREIGN KEY (provider_id) REFERENCES identity_providers(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE auth_states (
    state TEXT PRIMARY KEY,
    provider_id TEXT NOT NULL,
    nonce TEXT NOT NULL,
    code_verifier TEXT NOT NULL,
    redirect_url TEXT NOT NULL,
    expires_at DATETIME NOT NULL,
    FOREIGN KEY (provider_id) REFERENCES identity_providers(id) ON DELETE CASCADE
);

CREATE TABLE revoked_tokens (
    id TEXT PRIMARY KEY,
    expires_at DATETIME NOT NULL
);

CREATE TABLE registration_tokens (
    id TEXT PRIMARY KEY,
    value_hash TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    max_uses INTEGER NOT NULL DEFAULT 0,
    current_uses INTEGER NOT NULL DEFAULT 0,
    expires_at DATETIME NOT NULL,
    created_at DATETIME NOT NULL,
    disabled BOOLEAN NOT NULL DEFAULT FALSE
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
    last_seen_at DATETIME
);

CREATE TABLE actions (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    type INTEGER NOT NULL,
    desired_state INTEGER NOT NULL,
    timeout_seconds INTEGER NOT NULL DEFAULT 0,
    interval_hours INTEGER NOT NULL DEFAULT 0,
    run_on_assign BOOLEAN NOT NULL DEFAULT FALSE,
    skip_if_unchanged BOOLEAN NOT NULL DEFAULT FALSE,
    package_name TEXT NOT NULL DEFAULT '',
    package_version TEXT NOT NULL DEFAULT '',
    shell_script TEXT NOT NULL DEFAULT '',
    shell_interpreter TEXT NOT NULL DEFAULT '',
    shell_working_directory TEXT NOT NULL DEFAULT '',
    shell_environment_json TEXT NOT NULL DEFAULT '{}',
    shell_detection_script TEXT NOT NULL DEFAULT '',
    shell_is_compliance BOOLEAN NOT NULL DEFAULT FALSE,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);

CREATE TABLE device_groups (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL
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
    status INTEGER NOT NULL,
    error TEXT NOT NULL DEFAULT '',
    output_exit_code INTEGER NOT NULL DEFAULT 0,
    output_stdout TEXT NOT NULL DEFAULT '',
    output_stderr TEXT NOT NULL DEFAULT '',
    completed_at DATETIME NOT NULL,
    compliant BOOLEAN NOT NULL DEFAULT FALSE,
    detection_exit_code INTEGER NOT NULL DEFAULT 0,
    detection_stdout TEXT NOT NULL DEFAULT '',
    detection_stderr TEXT NOT NULL DEFAULT '',
    is_compliance BOOLEAN NOT NULL DEFAULT FALSE,
    FOREIGN KEY (device_id) REFERENCES devices(id) ON DELETE CASCADE,
    FOREIGN KEY (action_id) REFERENCES actions(id) ON DELETE CASCADE
);

CREATE INDEX execution_results_device_time ON execution_results(device_id, completed_at DESC);

CREATE TABLE audit_events (
    id TEXT PRIMARY KEY,
    event_type TEXT NOT NULL,
    stream_type TEXT NOT NULL,
    stream_id TEXT NOT NULL,
    actor_type TEXT NOT NULL,
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
DROP TABLE revoked_tokens;
DROP TABLE auth_states;
DROP TABLE identity_links;
DROP TABLE users;
DROP TABLE identity_providers;
