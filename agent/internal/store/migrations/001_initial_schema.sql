-- +goose Up

CREATE TABLE settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE scheduled_work (
    work_id TEXT PRIMARY KEY,
    run_id TEXT UNIQUE,
    manifest_blob BLOB NOT NULL,
    retired BOOLEAN NOT NULL DEFAULT FALSE,
    received_at DATETIME NOT NULL,
    last_executed_at DATETIME,
    next_execute_at DATETIME NOT NULL,
    run_started_at DATETIME,
    run_in_progress BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE INDEX idx_scheduled_work_due ON scheduled_work(next_execute_at);

CREATE TABLE scheduled_work_occurrences (
    work_id TEXT NOT NULL,
    occurrence_id TEXT NOT NULL,
    position INTEGER NOT NULL,
    action_id TEXT NOT NULL,
    state TEXT NOT NULL DEFAULT 'PENDING'
        CHECK (state IN ('PENDING', 'STARTED', 'SUCCESS', 'FAILED', 'INDETERMINATE')),
    started_at DATETIME,
    completed_at DATETIME,
    result_status INTEGER,
    result_error TEXT NOT NULL DEFAULT '',
    last_result_hash TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (work_id, occurrence_id),
    UNIQUE (work_id, position),
    FOREIGN KEY (work_id) REFERENCES scheduled_work(work_id) ON DELETE CASCADE
);

CREATE TABLE result_outbox (
    sequence INTEGER PRIMARY KEY AUTOINCREMENT,
    id TEXT NOT NULL UNIQUE,
    kind TEXT NOT NULL CHECK (kind IN ('ACTION', 'MANIFEST')),
    payload BLOB NOT NULL,
    created_at DATETIME NOT NULL,
    synced BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE INDEX idx_result_outbox_pending ON result_outbox(sequence) WHERE synced = FALSE;

CREATE TABLE luks_state (
    action_id TEXT PRIMARY KEY,
    device_path TEXT NOT NULL DEFAULT '',
    ownership_taken BOOLEAN NOT NULL DEFAULT FALSE,
    device_key_type TEXT NOT NULL DEFAULT 'none',
    last_rotated_at TEXT NOT NULL DEFAULT ''
);

CREATE TABLE luks_user_passphrase_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    action_id TEXT NOT NULL,
    passphrase_hash TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_luks_passphrase_history_action ON luks_user_passphrase_history(action_id);

CREATE TABLE lps_state (
    action_id TEXT NOT NULL,
    username TEXT NOT NULL,
    last_rotated_at TEXT NOT NULL DEFAULT '',
    password_hash TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (action_id, username)
);

-- +goose Down

DROP TABLE lps_state;
DROP TABLE luks_user_passphrase_history;
DROP TABLE luks_state;
DROP TABLE result_outbox;
DROP TABLE scheduled_work_occurrences;
DROP TABLE scheduled_work;
DROP TABLE settings;
