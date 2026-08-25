package store_test

import (
	"context"
	"strings"
	"testing"
	"time"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/server/internal/agentsecrets"
	"github.com/manchtools/cadestro/server/internal/crypto"
	"github.com/manchtools/cadestro/server/internal/store"
	"github.com/manchtools/cadestro/server/internal/testdb"
	"github.com/stretchr/testify/require"
)

func newAgentSecretFixture(t *testing.T) (*store.Store, *testdb.DB, *crypto.Encryptor, string, string, string) {
	t.Helper()
	st, raw := setupSQLite(t)
	atRest, err := crypto.NewEncryptor(strings.Repeat("01", 32))
	require.NoError(t, err)
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	device, luks, lps := newID(), newID(), newID()
	_, err = raw.Exec(context.Background(),
		`INSERT INTO devices (id, hostname, agent_version, registered_at) VALUES ($1, 'device', 'v1', $2)`, device, now)
	require.NoError(t, err)
	_, err = raw.Exec(context.Background(), `
		INSERT INTO actions (id, name, action_type, params, created_at, updated_at)
		VALUES ($1, 'disk encryption', $3, '{}', $4, $4), ($2, 'local passwords', $5, '{}', $4, $4)`,
		luks, lps, int32(cadestrov1.ActionType_ACTION_TYPE_ENCRYPTION), now,
		int32(cadestrov1.ActionType_ACTION_TYPE_LPS))
	require.NoError(t, err)
	return st, raw, atRest, device, luks, lps
}

func TestAgentSecretsLuksUsesGenericCiphertextAndPlaintextWireBytes(t *testing.T) {
	st, _, atRest, device, action, _ := newAgentSecretFixture(t)
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	svc := agentsecrets.New(agentsecrets.Config{Store: st, AtRest: atRest, Now: func() time.Time { return now }})
	ctx := context.Background()
	const passphrase = "correct horse battery staple"
	_, err := svc.StoreLuksKey(ctx, device, &cadestrov1.StoreLuksKeyRequest{
		ActionId: &cadestrov1.ActionId{Value: action}, DevicePath: "/dev/vda3", Passphrase: []byte(passphrase),
		RotationReason: cadestrov1.RotationReason_ROTATION_REASON_INITIAL,
	})
	require.NoError(t, err)
	row, err := st.GetCurrentLuksKeyForAgent(ctx, device, action)
	require.NoError(t, err)
	secret, err := st.GetDeviceSecret(ctx, row.ID)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(secret.Ciphertext, "enc:v1:"))
	opened, err := atRest.DecryptWithContext(secret.Ciphertext,
		crypto.DeviceSecretAAD(row.ID, device, "luks", action, 1))
	require.NoError(t, err)
	require.Equal(t, passphrase, opened)
	got, err := svc.GetLuksKey(ctx, device, &cadestrov1.GetLuksKeyRequest{ActionId: &cadestrov1.ActionId{Value: action}})
	require.NoError(t, err)
	require.Equal(t, []byte(passphrase), got.Passphrase)
}

func TestAgentSecretsLpsBatchIsAtomicAndStoresNoPlaintext(t *testing.T) {
	st, raw, atRest, device, _, action := newAgentSecretFixture(t)
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	svc := agentsecrets.New(agentsecrets.Config{Store: st, AtRest: atRest, Now: func() time.Time { return now }})
	ctx := context.Background()
	request := &cadestrov1.StoreLpsPasswordsRequest{ActionId: &cadestrov1.ActionId{Value: action}, Rotations: []*cadestrov1.LpsPasswordRotation{
		{Username: "alice", Password: []byte("alice-secret"), RotatedAt: now.Format(time.RFC3339Nano), Reason: cadestrov1.RotationReason_ROTATION_REASON_SCHEDULED},
		{Username: "bob", Password: []byte("bob-secret"), RotatedAt: now.Format(time.RFC3339Nano), Reason: cadestrov1.RotationReason_ROTATION_REASON_SCHEDULED},
	}}
	_, err := svc.StoreLpsPasswords(ctx, device, request)
	require.NoError(t, err)
	var count int
	require.NoError(t, raw.QueryRow(ctx, `SELECT count(*) FROM lps_passwords`).Scan(&count))
	require.Equal(t, 2, count)
	var ciphertext string
	require.NoError(t, raw.QueryRow(ctx, `SELECT ds.ciphertext FROM lps_passwords p JOIN device_secrets ds ON ds.id = p.id WHERE p.username = 'alice'`).Scan(&ciphertext))
	require.True(t, strings.HasPrefix(ciphertext, "enc:v1:"))
	require.NotContains(t, ciphertext, "alice-secret")
}
