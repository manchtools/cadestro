-- Manual forward migration for databases created before the reusable-token
-- cutover. This repository's upgrade model is operator-run SQL; do not edit a
-- released schema file or guess missing enrollment provenance.
--
-- Run the reports first. Resolve every row they return before continuing:
--   SELECT id, name FROM tokens WHERE expires_at IS NULL;
--   SELECT id FROM devices WHERE registration_token_id IS NULL
--       AND registered_at IS NOT NULL;
--   SELECT id, owner_id FROM tokens WHERE owner_id IS NOT NULL;
--   SELECT t.id, t.current_uses,
--       (SELECT COUNT(*) FROM devices d WHERE d.registration_token_id = t.id)
--       AS derived_uses FROM tokens t
--       WHERE t.current_uses IS NULL OR t.current_uses <> (SELECT COUNT(*)
--       FROM devices d WHERE d.registration_token_id = t.id);
-- A token with no expiry cannot be safely converted to the required TTL model.
-- A device with no token relation cannot be assigned to a token by inference.
-- A non-NULL owner cannot be carried into the ownerless model without guessing;
-- export it for a separately reviewed manual device assignment first.
-- Keep those rows in a separately reviewed quarantine and stop this script.
-- Execute with SQLite's fail-closed mode so a guard error aborts before any
-- table replacement: `sqlite3 -bail cadestro.db < upgrade-enrollment-tokens.sql`.

PRAGMA foreign_keys = OFF;
BEGIN;

-- Evidence for the operator: old counters are compared with immutable device
-- provenance before either table is changed. A non-empty result is not
-- silently corrected; investigate it and stop.
SELECT t.id, t.current_uses,
       (SELECT COUNT(*) FROM devices d WHERE d.registration_token_id = t.id) AS derived_uses
FROM tokens t
WHERE t.current_uses <> (SELECT COUNT(*) FROM devices d WHERE d.registration_token_id = t.id);

CREATE TEMP TABLE enrollment_cutover_guard (
    ok integer NOT NULL CHECK (ok = 1)
);
INSERT INTO enrollment_cutover_guard(ok)
SELECT CASE WHEN EXISTS (
    SELECT 1 FROM tokens WHERE expires_at IS NULL
) OR EXISTS (
    SELECT 1 FROM devices d
    WHERE d.registered_at IS NOT NULL AND d.registration_token_id IS NULL
) OR EXISTS (
    SELECT 1 FROM tokens WHERE owner_id IS NOT NULL
) OR EXISTS (
    SELECT 1 FROM tokens t
    WHERE t.current_uses IS NULL
       OR t.current_uses <> (SELECT COUNT(*) FROM devices d WHERE d.registration_token_id = t.id)
) THEN 0 ELSE 1 END;

CREATE TABLE tokens_cutover (
    id         text PRIMARY KEY,
    value_hash text NOT NULL UNIQUE,
    name       text NOT NULL DEFAULT '',
    max_uses   integer NOT NULL DEFAULT 0,
    expires_at timestamp NOT NULL,
    created_at timestamp,
    created_by text NOT NULL DEFAULT '',
    disabled   boolean NOT NULL DEFAULT false,
    is_deleted boolean NOT NULL DEFAULT false
);

-- This INSERT intentionally fails if an operator skipped the NULL-expiry
-- report. IDs, bearer digests, expiry, revocation/disabled state, and soft
-- deletion are copied without reinterpretation. current_uses/one_time/owner_id
-- are not copied: use is derived only from devices.registration_token_id.
INSERT INTO tokens_cutover
    (id, value_hash, name, max_uses, expires_at, created_at, created_by, disabled, is_deleted)
SELECT id, value_hash, name,
       CASE WHEN one_time THEN 1 ELSE max_uses END,
       expires_at, created_at, created_by, disabled, is_deleted
FROM tokens
;

DROP TABLE tokens;
ALTER TABLE tokens_cutover RENAME TO tokens;

-- New enrollment rows must provide the CSR's Ed25519 key. Existing rows stay
-- nullable for manual review; the unique index covers even soft-deleted rows
-- so an old identity cannot be silently resurrected.
ALTER TABLE devices ADD COLUMN enrollment_identity_public_key blob;
ALTER TABLE devices ADD COLUMN certificate_pem blob;
CREATE UNIQUE INDEX idx_devices_enrollment_identity
    ON devices(enrollment_identity_public_key)
    WHERE enrollment_identity_public_key IS NOT NULL;

-- Legacy devices used ON DELETE SET NULL. Install the guard before commit so
-- a later trigger error cannot leave the rebuilt token table unprotected.
CREATE TRIGGER devices_keep_enrollment_token
BEFORE DELETE ON tokens
WHEN EXISTS (SELECT 1 FROM devices WHERE registration_token_id = OLD.id)
BEGIN
    SELECT RAISE(ABORT, 'enrollment token provenance is immutable');
END;

COMMIT;
PRAGMA foreign_keys = ON;

-- Verify before opening the upgraded database:
--   PRAGMA foreign_key_check;
--   SELECT id FROM devices WHERE registration_token_id IS NULL AND registered_at IS NOT NULL;
