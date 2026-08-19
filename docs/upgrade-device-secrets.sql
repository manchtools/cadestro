-- Device-secret cutover for schema-v1 databases.
-- Run with sqlite3 -bail while control is stopped, after a verified backup.
-- This DDL is deliberately non-destructive. After it succeeds, run the
-- stopped-control command
-- `CADESTRO_DATABASE_PATH=... CADESTRO_ENCRYPTION_KEY=... cadestro
-- migrate-device-secrets`. That command authenticates every legacy LUKS/LPS
-- row, inserts one SDK-AEAD row per secret, and rebuilds the typed tables
-- without their secret columns in the same transaction.
PRAGMA foreign_keys = ON;
BEGIN IMMEDIATE;

CREATE TABLE IF NOT EXISTS device_secrets (
    id          text PRIMARY KEY,
    device_id   text NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    kind        text NOT NULL CHECK (kind <> ''),
    subject     text NOT NULL CHECK (subject <> ''),
    version     integer NOT NULL CHECK (version > 0),
    ciphertext  text NOT NULL CHECK (ciphertext LIKE 'enc:v1:%'),
    created_at  timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_device_secrets_owner
    ON device_secrets(device_id, kind, subject, version);

COMMIT;

-- Validation is an application-level AEAD open under
-- DeviceSecretAAD(row_id, device_id, kind, subject, version). Keep
-- lps_passwords and luks_keys until that validation succeeds for every row.
