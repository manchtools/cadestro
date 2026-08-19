package store_test

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/manchtools/cadestro/server/internal/testdb"
)

func seedCertificateLifecycleDevice(t *testing.T, raw *testdb.DB, id string, active, pending any) {
	t.Helper()
	_, err := raw.Exec(context.Background(), `
		INSERT INTO devices (id, agent_sealing_public_key, certificate_pem,
			active_cert_serial, pending_certificate_pem, pending_cert_serial,
			cert_fingerprint, cert_not_after)
		VALUES (?, zeroblob(32), ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		id, []byte("active"), active, pending, pending, "legacy-fingerprint")
	require.NoError(t, err)
}

func TestLegacyCertificateBridgeIsAtomicAndIdempotent(t *testing.T) {
	_, raw := setupSQLite(t)
	seedCertificateLifecycleDevice(t, raw, newID(), nil, nil)
	var id string
	require.NoError(t, raw.QueryRow(context.Background(), `SELECT id FROM devices WHERE cert_fingerprint = 'legacy-fingerprint'`).Scan(&id))

	var wg sync.WaitGroup
	results := make(chan int64, 2)
	for _, serial := range []string{"serial-a", "serial-b"} {
		wg.Add(1)
		go func(serial string) {
			defer wg.Done()
			tag, err := raw.Exec(context.Background(), `
				UPDATE devices SET active_cert_serial = ?, cert_fingerprint = NULL, cert_not_after = NULL
				WHERE id = ? AND active_cert_serial IS NULL AND cert_fingerprint = 'legacy-fingerprint'`, serial, id)
			require.NoError(t, err)
			results <- tag.RowsAffected()
		}(serial)
	}
	wg.Wait()
	close(results)
	var changed int
	for rows := range results {
		changed += int(rows)
	}
	require.Equal(t, 1, changed)
	var serial string
	var fingerprint any
	require.NoError(t, raw.QueryRow(context.Background(), `SELECT active_cert_serial, cert_fingerprint FROM devices WHERE id = ?`, id).Scan(&serial, &fingerprint))
	require.NotEmpty(t, serial)
	require.Nil(t, fingerprint)
}

func TestPendingCertificatePromotionReplacesOnlyAfterMatchingHelloSerial(t *testing.T) {
	_, raw := setupSQLite(t)
	id := newID()
	seedCertificateLifecycleDevice(t, raw, id, "active-a", "pending-b")
	wrong, err := raw.Exec(context.Background(), `UPDATE devices SET certificate_pem = pending_certificate_pem, active_cert_serial = pending_cert_serial, pending_certificate_pem = NULL, pending_cert_serial = NULL WHERE id = ? AND pending_cert_serial = 'wrong'`, id)
	require.NoError(t, err)
	require.Zero(t, wrong.RowsAffected())
	winning, err := raw.Exec(context.Background(), `UPDATE devices SET certificate_pem = pending_certificate_pem, cert_fingerprint = NULL, cert_not_after = NULL, active_cert_serial = pending_cert_serial, pending_certificate_pem = NULL, pending_cert_serial = NULL WHERE id = ? AND pending_cert_serial = 'pending-b'`, id)
	require.NoError(t, err)
	require.EqualValues(t, 1, winning.RowsAffected())
	var active, pending any
	require.NoError(t, raw.QueryRow(context.Background(), `SELECT active_cert_serial, pending_cert_serial FROM devices WHERE id = ?`, id).Scan(&active, &pending))
	require.Equal(t, "pending-b", active)
	require.Nil(t, pending)
}
