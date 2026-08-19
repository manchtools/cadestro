package store

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestCertificateLifecyclePostureRequiresCutoverColumns(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE devices (id text); PRAGMA user_version = 1`); err != nil {
		t.Fatal(err)
	}
	err = checkCertificateLifecyclePosture(context.Background(), db)
	if err == nil || !strings.Contains(err.Error(), "upgrade-certificate-lifecycle.sql") {
		t.Fatalf("missing posture error = %v", err)
	}
}

func TestCertificateLifecyclePostureAcceptsCutoverColumns(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE devices (
		id text, active_cert_serial text, pending_certificate_pem blob,
		pending_cert_serial text
	);
	CREATE TRIGGER devices_certificate_lifecycle_pair BEFORE INSERT ON devices BEGIN SELECT 1; END;
	CREATE TRIGGER devices_certificate_lifecycle_pair_update BEFORE UPDATE ON devices BEGIN SELECT 1; END;
`); err != nil {
		t.Fatal(err)
	}
	if err := checkCertificateLifecyclePosture(context.Background(), db); err != nil {
		t.Fatalf("complete posture rejected: %v", err)
	}
}
