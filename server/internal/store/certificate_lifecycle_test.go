package store_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/manchtools/cadestro/server/internal/testdb"
)

func seedCertificateLifecycleDevice(t *testing.T, raw *testdb.DB, id string, active, pending any) {
	t.Helper()
	_, err := raw.Exec(context.Background(), `
		INSERT INTO devices (id, certificate_pem,
			active_cert_serial, pending_certificate_pem, pending_cert_serial)
		VALUES (?, ?, ?, ?, ?)`,
		id, []byte("active"), active, pending, pending)
	require.NoError(t, err)
}

func TestPendingCertificatePromotionReplacesOnlyAfterMatchingHelloSerial(t *testing.T) {
	_, raw := setupSQLite(t)
	id := newID()
	seedCertificateLifecycleDevice(t, raw, id, "active-a", "pending-b")
	wrong, err := raw.Exec(context.Background(), `UPDATE devices SET certificate_pem = pending_certificate_pem, active_cert_serial = pending_cert_serial, pending_certificate_pem = NULL, pending_cert_serial = NULL WHERE id = ? AND pending_cert_serial = 'wrong'`, id)
	require.NoError(t, err)
	require.Zero(t, wrong.RowsAffected())
	winning, err := raw.Exec(context.Background(), `UPDATE devices SET certificate_pem = pending_certificate_pem, active_cert_serial = pending_cert_serial, pending_certificate_pem = NULL, pending_cert_serial = NULL WHERE id = ? AND pending_cert_serial = 'pending-b'`, id)
	require.NoError(t, err)
	require.EqualValues(t, 1, winning.RowsAffected())
	var active, pending any
	require.NoError(t, raw.QueryRow(context.Background(), `SELECT active_cert_serial, pending_cert_serial FROM devices WHERE id = ?`, id).Scan(&active, &pending))
	require.Equal(t, "pending-b", active)
	require.Nil(t, pending)
}
