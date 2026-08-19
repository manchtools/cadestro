-- +goose Up

-- One local scheduled-work engine serves both transport deliveries and
-- assignment policy. The discriminator is a domain fact, not a second
-- delivery state machine.
ALTER TABLE manifest_deliveries RENAME TO scheduled_work;
ALTER TABLE manifest_occurrences RENAME TO scheduled_work_occurrences;
ALTER TABLE reboot_markers RENAME TO scheduled_work_reboots;
ALTER TABLE scheduled_work ADD COLUMN kind TEXT NOT NULL DEFAULT 'delivery'
    CHECK (kind IN ('delivery', 'policy'));
ALTER TABLE scheduled_work ADD COLUMN retired BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE scheduled_work ADD COLUMN run_id TEXT;
UPDATE scheduled_work SET run_id = delivery_id WHERE run_id IS NULL;
CREATE UNIQUE INDEX idx_scheduled_work_run ON scheduled_work(run_id);

-- +goose Down

ALTER TABLE scheduled_work DROP COLUMN retired;
ALTER TABLE scheduled_work DROP COLUMN kind;
DROP INDEX idx_scheduled_work_run;
ALTER TABLE scheduled_work DROP COLUMN run_id;
ALTER TABLE scheduled_work_reboots RENAME TO reboot_markers;
ALTER TABLE scheduled_work_occurrences RENAME TO manifest_occurrences;
ALTER TABLE scheduled_work RENAME TO manifest_deliveries;
