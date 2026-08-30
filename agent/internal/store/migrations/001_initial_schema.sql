-- +goose Up

CREATE TABLE settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
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
    run_in_progress BOOLEAN NOT NULL DEFAULT FALSE,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
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
    last_result_hash TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
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
    updated_at DATETIME NOT NULL,
    synced BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE INDEX idx_result_outbox_pending ON result_outbox(sequence) WHERE synced = FALSE;

-- +goose Down

DROP TABLE result_outbox;
DROP TABLE scheduled_work_occurrences;
DROP TABLE scheduled_work;
DROP TABLE settings;
