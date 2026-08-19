package store_test

import (
	"context"
	"strings"
	"testing"
	"time"

	pmv1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/server/internal/agentsecrets"
	pmcrypto "github.com/manchtools/cadestro/server/internal/crypto"
	"github.com/manchtools/cadestro/server/internal/store"
	"github.com/manchtools/cadestro/server/internal/testdb"
	"github.com/stretchr/testify/require"
)

type legacySecretsFixture struct {
	store                           *store.Store
	raw                             *testdb.DB
	atRest                          *pmcrypto.Encryptor
	deviceID, luksAction, lpsAction string
	luksID, lpsID                   string
}

func setupLegacySecrets(t *testing.T, corruptLPS bool) legacySecretsFixture {
	t.Helper()
	st, raw := setupSQLite(t)
	ctx := context.Background()
	atRest, err := pmcrypto.NewEncryptor(strings.Repeat("01", 32))
	require.NoError(t, err)

	for _, statement := range []string{
		`DROP TABLE lps_passwords`,
		`DROP TABLE luks_keys`,
		`DROP TABLE device_secrets`,
		`CREATE TABLE lps_passwords (
            id text PRIMARY KEY,
            device_id text NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
            action_id text NOT NULL REFERENCES actions(id),
            username text NOT NULL,
            password text NOT NULL,
            rotated_at timestamp NOT NULL,
            rotation_reason text NOT NULL DEFAULT 'scheduled',
            is_current boolean NOT NULL DEFAULT true,
            created_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP
        )`,
		`CREATE INDEX idx_lps_passwords_device ON lps_passwords(device_id, is_current)`,
		`CREATE INDEX idx_lps_passwords_action_device ON lps_passwords(action_id, device_id)`,
		`CREATE INDEX idx_lps_passwords_username ON lps_passwords(device_id, action_id, username, is_current)`,
		`CREATE TABLE luks_keys (
            id text PRIMARY KEY,
            device_id text NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
            action_id text NOT NULL REFERENCES actions(id),
            device_path text NOT NULL,
            passphrase text NOT NULL,
            rotated_at timestamp NOT NULL,
            rotation_reason text NOT NULL DEFAULT 'scheduled',
            is_current boolean NOT NULL DEFAULT true,
            created_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
            revocation_status text,
            revocation_error text,
            revocation_at timestamp
        )`,
		`CREATE INDEX idx_luks_keys_device ON luks_keys(device_id, is_current)`,
		`CREATE INDEX idx_luks_keys_action_device ON luks_keys(action_id, device_id)`,
		`CREATE INDEX idx_luks_keys_current ON luks_keys(device_id, action_id, device_path, is_current)`,
		`CREATE TABLE device_secrets (
            id text PRIMARY KEY,
            device_id text NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
            kind text NOT NULL CHECK (kind <> ''),
            subject text NOT NULL CHECK (subject <> ''),
            version integer NOT NULL CHECK (version > 0),
            ciphertext text NOT NULL CHECK (ciphertext LIKE 'enc:v1:%'),
            created_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP
        )`,
		`CREATE INDEX idx_device_secrets_owner ON device_secrets(device_id, kind, subject, version)`,
	} {
		_, err := raw.Exec(ctx, statement)
		require.NoError(t, err)
	}

	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	fixture := legacySecretsFixture{
		store: st, raw: raw, atRest: atRest,
		deviceID: newID(), luksAction: newID(), lpsAction: newID(),
		luksID: newID(), lpsID: newID(),
	}
	_, err = raw.Exec(ctx, `INSERT INTO devices (id, hostname, agent_version, registered_at) VALUES ($1, 'legacy', 'v1', $2)`, fixture.deviceID, now)
	require.NoError(t, err)
	_, err = raw.Exec(ctx, `INSERT INTO actions (id, name, action_type, params) VALUES ($1, 'luks', $3, '{}'), ($2, 'lps', $4, '{}')`, fixture.luksAction, fixture.lpsAction, int32(pmv1.ActionType_ACTION_TYPE_ENCRYPTION), int32(pmv1.ActionType_ACTION_TYPE_LPS))
	require.NoError(t, err)
	luksCiphertext, err := atRest.EncryptWithContext("legacy-luks", pmcrypto.LegacySecretAADForRow(fixture.deviceID, fixture.luksAction, "luks", fixture.luksID))
	require.NoError(t, err)
	lpsCiphertext, err := atRest.EncryptWithContext("legacy-lps", pmcrypto.LegacySecretAADForRow(fixture.deviceID, fixture.lpsAction, "lps", fixture.lpsID))
	require.NoError(t, err)
	if corruptLPS {
		lpsCiphertext = "enc:v1:not-base64"
	}
	_, err = raw.Exec(ctx, `INSERT INTO luks_keys (id, device_id, action_id, device_path, passphrase, rotated_at) VALUES ($1, $2, $3, '/dev/vda3', $4, $5)`, fixture.luksID, fixture.deviceID, fixture.luksAction, luksCiphertext, now)
	require.NoError(t, err)
	_, err = raw.Exec(ctx, `INSERT INTO lps_passwords (id, device_id, action_id, username, password, rotated_at) VALUES ($1, $2, $3, 'alice', $4, $5)`, fixture.lpsID, fixture.deviceID, fixture.lpsAction, lpsCiphertext, now)
	require.NoError(t, err)
	return fixture
}

func TestMigrateDeviceSecretRowsReencryptsAndRebuildsMetadata(t *testing.T) {
	fixture := setupLegacySecrets(t, false)
	ctx := context.Background()
	require.NoError(t, fixture.store.MigrateDeviceSecretRows(ctx, fixture.atRest))

	for table, removed := range map[string][]string{
		"lps_passwords": {"device_id", "action_id", "password"},
		"luks_keys":     {"device_id", "action_id", "passphrase"},
	} {
		columns := scanStrings(t, fixture.raw, `SELECT name FROM pragma_table_info(?)`, table)
		for _, column := range removed {
			require.NotContains(t, columns, column)
		}
	}
	for _, want := range []struct {
		id, kind, subject, plaintext string
	}{
		{fixture.luksID, "luks", fixture.luksAction, "legacy-luks"},
		{fixture.lpsID, "lps", fixture.lpsAction, "legacy-lps"},
	} {
		secret, err := fixture.store.GetDeviceSecret(ctx, want.id)
		require.NoError(t, err)
		require.Equal(t, fixture.deviceID, secret.DeviceID)
		require.Equal(t, want.kind, secret.Kind)
		require.Equal(t, want.subject, secret.Subject)
		plaintext, err := fixture.atRest.DecryptWithContext(secret.Ciphertext,
			pmcrypto.DeviceSecretAAD(secret.ID, secret.DeviceID, secret.Kind, secret.Subject, uint32(secret.Version)))
		require.NoError(t, err)
		require.Equal(t, want.plaintext, plaintext)
	}

	service := agentsecrets.New(agentsecrets.Config{Store: fixture.store, AtRest: fixture.atRest})
	_, err := service.StoreLuksKey(ctx, fixture.deviceID, &pmv1.StoreLuksKeyRequest{
		ActionId: fixture.luksAction, DevicePath: "/dev/vda3", Passphrase: []byte("new-luks"),
		RotationReason: pmv1.RotationReason_ROTATION_REASON_SCHEDULED,
	})
	require.NoError(t, err, "post-migration LUKS writes must use the rebuilt metadata table")
	_, err = service.StoreLpsPasswords(ctx, fixture.deviceID, &pmv1.StoreLpsPasswordsRequest{
		ActionId: fixture.lpsAction,
		Rotations: []*pmv1.LpsPasswordRotation{{
			Username: "alice", Password: []byte("new-lps"), RotatedAt: time.Now().UTC().Format(time.RFC3339Nano),
			Reason: pmv1.RotationReason_ROTATION_REASON_SCHEDULED,
		}},
	})
	require.NoError(t, err, "post-migration LPS writes must use the rebuilt metadata table")

	var count int
	require.NoError(t, fixture.raw.QueryRow(ctx, `SELECT count(*) FROM device_secrets`).Scan(&count))
	require.Equal(t, 4, count)
	require.Error(t, fixture.store.MigrateDeviceSecretRows(ctx, fixture.atRest), "the cutover is one-shot")
	require.NoError(t, fixture.raw.QueryRow(ctx, `SELECT count(*) FROM device_secrets`).Scan(&count))
	require.Equal(t, 4, count, "a repeated attempt must not mutate the cut-over database")
}

func TestMigrateDeviceSecretRowsRollsBackOnBadCiphertext(t *testing.T) {
	fixture := setupLegacySecrets(t, true)
	ctx := context.Background()
	require.Error(t, fixture.store.MigrateDeviceSecretRows(ctx, fixture.atRest))

	var count int
	require.NoError(t, fixture.raw.QueryRow(ctx, `SELECT count(*) FROM device_secrets`).Scan(&count))
	require.Zero(t, count)
	require.Contains(t, scanStrings(t, fixture.raw, `SELECT name FROM pragma_table_info('lps_passwords')`), "password")
	require.Contains(t, scanStrings(t, fixture.raw, `SELECT name FROM pragma_table_info('luks_keys')`), "passphrase")
	require.NoError(t, fixture.raw.QueryRow(ctx, `SELECT count(*) FROM lps_passwords`).Scan(&count))
	require.Equal(t, 1, count)
	require.NoError(t, fixture.raw.QueryRow(ctx, `SELECT height FROM audit_chain_head WHERE stream = 'control'`).Scan(&count))
	require.Zero(t, count, "the failed cutover must not leave an audit operation outside its rolled-back mutation")
}
