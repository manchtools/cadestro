package store_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"log/slog"
	"math/big"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/manchtools/cadestro/server/internal/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pmv1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/contract/gen/go/cadestro/v1/cadestrov1connect"
	"github.com/manchtools/cadestro/server/internal/ca"
	"github.com/manchtools/cadestro/server/internal/enrollment"
	"github.com/manchtools/cadestro/server/internal/mtls"
	"github.com/manchtools/cadestro/server/internal/store"
)

type enrollmentFixture struct {
	t        *testing.T
	store    *store.Store
	raw      *testdb.DB
	handlers *enrollment.Handlers
	ca       *ca.CA
	now      time.Time
}

func newEnrollmentFixture(t *testing.T) *enrollmentFixture {
	t.Helper()
	st, raw := setupSQLite(t)
	now := time.Now().UTC().Truncate(time.Second)
	certPEM, keyPEM := enrollmentTestCA(t, now)
	certAuth, err := ca.NewFromPEM(certPEM, keyPEM, 24*time.Hour, ca.WithClock(func() time.Time { return now }))
	require.NoError(t, err)
	f := &enrollmentFixture{
		t: t, store: st, raw: raw, ca: certAuth, now: now,
	}
	f.handlers = enrollment.New(enrollment.Config{
		Store: st, CA: certAuth,
		Logger: slog.Default(),
		Now:    func() time.Time { return now }, ControlURL: "https://agents.example.test:8443",
	})
	return f
}

func enrollmentTestCA(t *testing.T, now time.Time) ([]byte, []byte) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Enrollment Test CA"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(7 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true, IsCA: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, public, private)
	require.NoError(t, err)
	keyDER, err := x509.MarshalPKCS8PrivateKey(private)
	require.NoError(t, err)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
}

func enrollmentCSR(t *testing.T, key ed25519.PrivateKey) []byte {
	t.Helper()
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{}, key)
	require.NoError(t, err)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der})
}

func newEnrollmentIdentity(t *testing.T) ([]byte, ed25519.PrivateKey) {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	return enrollmentCSR(t, key), key
}

func (f *enrollmentFixture) insertToken(plaintext string, maxUses int32, expiresAt time.Time) string {
	f.t.Helper()
	digest := sha256.Sum256([]byte(plaintext))
	id := newID()
	_, err := f.raw.Exec(context.Background(), `
		INSERT INTO tokens (
			id, value_hash, name, max_uses, expires_at, created_at, created_by
		) VALUES ($1, $2, 'enrollment', $3, $4, $5, 'test')`,
		id, hex.EncodeToString(digest[:]), maxUses, expiresAt, f.now)
	require.NoError(f.t, err)
	return id
}

func registerRequest(token string, csr []byte, _ byte) *connect.Request[pmv1.RegisterRequest] {
	return connect.NewRequest(&pmv1.RegisterRequest{
		Token: token, Hostname: "host-1", AgentVersion: "v1", Csr: csr,
	})
}

func renewalContext(t *testing.T, certPEM []byte, deviceID string) context.Context {
	t.Helper()
	block, _ := pem.Decode(certPEM)
	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	ctx := mtls.WithPeerCertificate(context.Background(), cert)
	return mtls.WithDeviceID(ctx, deviceID)
}

func TestEnrollment_ValidatesBeforeCredentialUse(t *testing.T) {
	f := newEnrollmentFixture(t)
	_, err := f.handlers.Register(context.Background(), connect.NewRequest(&pmv1.RegisterRequest{Token: "unknown"}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

	token := "still-usable"
	f.insertToken(token, 1, f.now.Add(time.Hour))
	_, err = f.handlers.Register(context.Background(), registerRequest(token, []byte("not a CSR"), 1))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	var devices int
	require.NoError(t, f.raw.QueryRow(context.Background(), `SELECT COUNT(*) FROM devices`).Scan(&devices))
	assert.Zero(t, devices, "an invalid CSR must not consume enrollment authority")
}

func TestEnrollment_RegisterCommitsOneAuditedDevice(t *testing.T) {
	f := newEnrollmentFixture(t)
	token := "reusable-token"
	tokenID := f.insertToken(token, 2, f.now.Add(time.Hour))
	csr, _ := newEnrollmentIdentity(t)

	resp, err := f.handlers.Register(context.Background(), registerRequest(token, csr, 0x24))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.DeviceId)
	deviceID := resp.Msg.DeviceId.Value
	assert.Equal(t, "https://agents.example.test:8443", resp.Msg.ControlUrl)
	assert.NotEmpty(t, resp.Msg.Certificate)
	assert.NotEmpty(t, resp.Msg.CaCert)

	var storedTokenID string
	require.NoError(t, f.raw.QueryRow(context.Background(), `
		SELECT d.registration_token_id FROM devices d WHERE d.id = $1`, deviceID).Scan(&storedTokenID))
	assert.Equal(t, tokenID, storedTokenID)
	var assignments int
	require.NoError(t, f.raw.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM device_assigned_users WHERE device_id = $1`, deviceID).Scan(&assignments))
	assert.Zero(t, assignments, "enrollment never assigns a human owner")

	op, err := latestOperationFor(t, f.store, f.raw, cadestrov1connect.ControlServiceRegisterProcedure)
	require.NoError(t, err)
	effects, err := f.store.ListAuditEffects(context.Background(), op.OperationID)
	require.NoError(t, err)
	assert.Len(t, effects, 1)

	retry, err := f.handlers.Register(context.Background(), registerRequest(token, csr, 0x24))
	require.NoError(t, err)
	assert.Equal(t, deviceID, retry.Msg.DeviceId.Value)
	assert.Equal(t, resp.Msg.Certificate, retry.Msg.Certificate, "retry returns the same certificate identity")
	_, err = f.handlers.Register(context.Background(), registerRequest(token, csr, 0x99))
	assert.NoError(t, err, "transport key state is not part of enrollment identity")
	var devices int
	require.NoError(t, f.raw.QueryRow(context.Background(), `SELECT COUNT(*) FROM devices`).Scan(&devices))
	assert.Equal(t, 1, devices)
}

func TestEnrollment_BoundedTokenWinsExactlyAtGlobalBoundary(t *testing.T) {
	f := newEnrollmentFixture(t)
	token := "raced-bounded-token"
	f.insertToken(token, 1, f.now.Add(time.Hour))

	const callers = 12
	var succeeded atomic.Int32
	var denied atomic.Int32
	requests := make([]*connect.Request[pmv1.RegisterRequest], callers)
	for i := range requests {
		csr, _ := newEnrollmentIdentity(t)
		requests[i] = registerRequest(token, csr, byte(0x33+i))
	}
	var wg sync.WaitGroup
	for i := range requests {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.handlers.Register(context.Background(), requests[i])
			switch connect.CodeOf(err) {
			case connect.CodeUnknown:
				if err == nil {
					succeeded.Add(1)
				}
			case connect.CodePermissionDenied:
				denied.Add(1)
			}
		}()
	}
	wg.Wait()
	assert.Equal(t, int32(1), succeeded.Load())
	assert.Equal(t, int32(callers-1), denied.Load())
	var devices int32
	require.NoError(t, f.raw.QueryRow(context.Background(), `SELECT COUNT(*) FROM devices`).Scan(&devices))
	assert.Equal(t, int32(1), devices)
}

func TestEnrollment_UnlimitedTokenEnrollsMultipleIdentities(t *testing.T) {
	f := newEnrollmentFixture(t)
	token := "unlimited-token"
	f.insertToken(token, 0, f.now.Add(time.Hour))
	firstCSR, _ := newEnrollmentIdentity(t)
	secondCSR, _ := newEnrollmentIdentity(t)
	first, err := f.handlers.Register(context.Background(), registerRequest(token, firstCSR, 0x41))
	require.NoError(t, err)
	second, err := f.handlers.Register(context.Background(), registerRequest(token, secondCSR, 0x42))
	require.NoError(t, err)
	assert.NotEqual(t, first.Msg.DeviceId.Value, second.Msg.DeviceId.Value)
	var devices int
	require.NoError(t, f.raw.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM devices WHERE registration_token_id = (SELECT id FROM tokens WHERE value_hash = $1)`,
		hex.EncodeToString(sha256Digest(token))).Scan(&devices))
	assert.Equal(t, 2, devices)
}

func TestEnrollment_ExpiredAndRevokedTokensFailClosed(t *testing.T) {
	for name, mutate := range map[string]func(*enrollmentFixture, string){
		"expired": func(f *enrollmentFixture, id string) {
			_, err := f.raw.Exec(context.Background(), `UPDATE tokens SET expires_at = $2 WHERE id = $1`, id, f.now.Add(-time.Minute))
			require.NoError(f.t, err)
		},
		"revoked": func(f *enrollmentFixture, id string) {
			_, err := f.raw.Exec(context.Background(), `UPDATE tokens SET disabled = TRUE WHERE id = $1`, id)
			require.NoError(f.t, err)
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newEnrollmentFixture(t)
			tokenID := f.insertToken(name+"-token", 0, f.now.Add(time.Hour))
			mutate(f, tokenID)
			csr, _ := newEnrollmentIdentity(t)
			_, err := f.handlers.Register(context.Background(), registerRequest(name+"-token", csr, 0x51))
			assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
		})
	}
}

func TestEnrollment_SoftDeletedIdentityCannotBeReusedOrRefundTokenUse(t *testing.T) {
	f := newEnrollmentFixture(t)
	token := "deleted-device-token"
	f.insertToken(token, 1, f.now.Add(time.Hour))
	csr, _ := newEnrollmentIdentity(t)
	registered, err := f.handlers.Register(context.Background(), registerRequest(token, csr, 0x61))
	require.NoError(t, err)
	_, err = f.raw.Exec(context.Background(), `UPDATE devices SET is_deleted = TRUE WHERE id = $1`, registered.Msg.DeviceId.Value)
	require.NoError(t, err)
	_, err = f.handlers.Register(context.Background(), registerRequest(token, csr, 0x62))
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
	otherCSR, _ := newEnrollmentIdentity(t)
	_, err = f.handlers.Register(context.Background(), registerRequest(token, otherCSR, 0x63))
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
	var devices int
	require.NoError(t, f.raw.QueryRow(context.Background(), `SELECT COUNT(*) FROM devices WHERE registration_token_id IS NOT NULL`).Scan(&devices))
	assert.Equal(t, 1, devices, "soft deletion does not refund the global token use")
}

func TestEnrollment_RenewalStagesPendingSuccessor(t *testing.T) {
	f := newEnrollmentFixture(t)
	token := "renew-token"
	f.insertToken(token, 1, f.now.Add(time.Hour))
	csr, identity := newEnrollmentIdentity(t)
	registered, err := f.handlers.Register(context.Background(), registerRequest(token, csr, 0x55))
	require.NoError(t, err)
	deviceID := registered.Msg.DeviceId.Value
	oldFingerprint, err := ca.FingerprintFromPEM(registered.Msg.Certificate)
	require.NoError(t, err)

	renewed, err := f.handlers.RenewCertificate(renewalContext(t, registered.Msg.Certificate, deviceID), connect.NewRequest(&pmv1.RenewCertificateRequest{
		Csr: enrollmentCSR(t, identity),
	}))
	require.NoError(t, err)
	newFingerprint, err := ca.FingerprintFromPEM(renewed.Msg.Certificate)
	require.NoError(t, err)
	assert.NotEqual(t, oldFingerprint, newFingerprint)
	assert.True(t, renewed.Msg.NotAfter.AsTime().Equal(f.now.Add(24*time.Hour)))

	var storedSerial string
	require.NoError(t, f.raw.QueryRow(context.Background(),
		`SELECT active_cert_serial FROM devices WHERE id = $1`, deviceID).Scan(&storedSerial))
	activeSerial, err := ca.SerialFromPEM(registered.Msg.Certificate)
	require.NoError(t, err)
	assert.Equal(t, activeSerial, storedSerial)
	var legacyFingerprint, legacyNotAfter any
	require.NoError(t, f.raw.QueryRow(context.Background(),
		`SELECT cert_fingerprint, cert_not_after FROM devices WHERE id = $1`, deviceID).Scan(&legacyFingerprint, &legacyNotAfter))
	assert.Nil(t, legacyFingerprint)
	assert.Nil(t, legacyNotAfter)
	var pendingSerial string
	require.NoError(t, f.raw.QueryRow(context.Background(),
		`SELECT pending_cert_serial FROM devices WHERE id = $1`, deviceID).Scan(&pendingSerial))
	assert.NotEmpty(t, pendingSerial)

	retry, err := f.handlers.RenewCertificate(renewalContext(t, registered.Msg.Certificate, deviceID), connect.NewRequest(&pmv1.RenewCertificateRequest{
		Csr: enrollmentCSR(t, identity),
	}))
	require.NoError(t, err)
	assert.Equal(t, renewed.Msg.Certificate, retry.Msg.Certificate)
}

func TestEnrollment_RenewalRequiresSameIdentityKey(t *testing.T) {
	f := newEnrollmentFixture(t)
	token := "key-binding-token"
	f.insertToken(token, 1, f.now.Add(time.Hour))
	csr, _ := newEnrollmentIdentity(t)
	registered, err := f.handlers.Register(context.Background(), registerRequest(token, csr, 0x66))
	require.NoError(t, err)
	otherCSR, _ := newEnrollmentIdentity(t)

	_, err = f.handlers.RenewCertificate(renewalContext(t, registered.Msg.Certificate, registered.Msg.DeviceId.Value), connect.NewRequest(&pmv1.RenewCertificateRequest{
		Csr: otherCSR,
	}))
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
}

func TestEnrollment_MountsExactSurface(t *testing.T) {
	f := newEnrollmentFixture(t)
	mux := http.NewServeMux()
	var mounted []string
	mounted = append(mounted, f.handlers.MountRegister(mux)...)
	mounted = append(mounted, f.handlers.MountRenewal(mux)...)
	assert.ElementsMatch(t, enrollment.MutationProcedures(), mounted)
	assert.Equal(t, []string{
		cadestrov1connect.ControlServiceRegisterProcedure,
		cadestrov1connect.ControlServiceRenewCertificateProcedure,
	}, enrollment.MutationProcedures())
}

func sha256Digest(value string) []byte {
	sum := sha256.Sum256([]byte(value))
	return sum[:]
}
