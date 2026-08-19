-- Certificate lifecycle cutover for schema version 1 databases.
-- Run with `sqlite3 -bail` after a verified backup and while control is
-- stopped. The guard refuses a partial or repeated run; it never guesses a
-- column's prior meaning or synthesizes a serial from copied application data.
PRAGMA foreign_keys = ON;
BEGIN IMMEDIATE;

CREATE TEMP TABLE certificate_lifecycle_guard (
    ok integer NOT NULL CHECK (ok = 1)
);
INSERT INTO certificate_lifecycle_guard(ok)
SELECT CASE WHEN
    (SELECT COUNT(*) FROM pragma_table_info('devices')
        WHERE name IN ('active_cert_serial', 'pending_certificate_pem', 'pending_cert_serial')) <> 0
    OR (SELECT COUNT(*) FROM pragma_table_info('devices')
        WHERE name IN ('certificate_pem', 'cert_fingerprint', 'cert_not_after')) <> 3
THEN 0 ELSE 1 END;

-- Evidence for the operator before the guarded ALTERs. Legacy identity is
-- bridged only after the peer presents the matching leaf over authenticated
-- mTLS; it must not be inferred here.
SELECT id, cert_fingerprint, cert_not_after
FROM devices
WHERE cert_fingerprint IS NOT NULL;

ALTER TABLE devices ADD COLUMN active_cert_serial text;
ALTER TABLE devices ADD COLUMN pending_certificate_pem blob;
ALTER TABLE devices ADD COLUMN pending_cert_serial text;

DROP TABLE IF EXISTS revoked_certificates;

CREATE TRIGGER devices_certificate_lifecycle_pair
BEFORE INSERT ON devices
WHEN (((NEW.active_cert_serial IS NULL) <> (NEW.certificate_pem IS NULL)) AND NEW.cert_fingerprint IS NULL)
  OR ((NEW.pending_cert_serial IS NULL) <> (NEW.pending_certificate_pem IS NULL))
BEGIN
    SELECT RAISE(ABORT, 'certificate serial and PEM must be stored together');
END;

CREATE TRIGGER devices_certificate_lifecycle_pair_update
BEFORE UPDATE OF active_cert_serial, certificate_pem, pending_cert_serial, pending_certificate_pem ON devices
WHEN (((NEW.active_cert_serial IS NULL) <> (NEW.certificate_pem IS NULL)) AND NEW.cert_fingerprint IS NULL)
  OR ((NEW.pending_cert_serial IS NULL) <> (NEW.pending_certificate_pem IS NULL))
BEGIN
    SELECT RAISE(ABORT, 'certificate serial and PEM must be stored together');
END;

COMMIT;
DROP TABLE certificate_lifecycle_guard;

-- Legacy cert_fingerprint/cert_not_after/certificate_pem are retained only
-- until every active row has bridged on its first authenticated connection.
-- Do not populate active_cert_serial from a copied PEM: the bridge verifies
-- the actual TLS leaf fingerprint and records its serial atomically.
