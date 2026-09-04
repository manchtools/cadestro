-- +goose Up
CREATE TABLE scheduled_work (work_id TEXT PRIMARY KEY, run_id TEXT UNIQUE, action_blob BLOB NOT NULL, run_action_digest BLOB, retired BOOLEAN NOT NULL DEFAULT FALSE, received_at TIMESTAMP NOT NULL, last_executed_at TIMESTAMP, next_execute_at TIMESTAMP NOT NULL, run_started_at TIMESTAMP, run_in_progress BOOLEAN NOT NULL DEFAULT FALSE, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP);
CREATE INDEX idx_scheduled_work_due ON scheduled_work(retired, run_in_progress, next_execute_at);
CREATE TABLE result_outbox (sequence INTEGER PRIMARY KEY AUTOINCREMENT, payload BLOB NOT NULL, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP);
-- +goose Down
DROP TABLE result_outbox;
DROP TABLE scheduled_work;
