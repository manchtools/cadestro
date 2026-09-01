package core

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/server/internal/ca"
	db "github.com/manchtools/cadestro/server/internal/store/generated"
)

func testEnrollmentCA(t *testing.T, now time.Time) *ca.CA {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	der, err := x509.CreateCertificate(rand.Reader, &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test-ca"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), IsCA: true,
		BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}, &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test-ca"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), IsCA: true,
		BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}, publicKey, privateKey)
	require.NoError(t, err)
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	require.NoError(t, err)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	result, err := ca.NewFromPEM(certPEM, keyPEM, time.Hour, ca.WithClock(func() time.Time { return now }))
	require.NoError(t, err)
	return result
}

func testEnrollmentCSR(t *testing.T) []byte {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "agent"}}, privateKey)
	require.NoError(t, err)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der})
}

func createEnrollmentToken(t *testing.T, service *Service, ctx context.Context, now time.Time, maxUses int32) string {
	t.Helper()
	response, err := service.CreateToken(ctx, connect.NewRequest(&cadestrov1.CreateTokenRequest{
		Name: "enrollment", MaxUses: maxUses, ExpiresAt: timestamppb.New(now.Add(time.Hour)),
	}))
	require.NoError(t, err)
	return response.Msg.Token.Value
}

func TestEnrollmentTokenLifecycle(t *testing.T) {
	service, ctx, now, _ := testService(t)
	service.ca = testEnrollmentCA(t, now)
	for _, test := range []struct {
		name     string
		maxUses  int32
		register int
		current  int32
	}{
		{name: "final finite use deletes token", maxUses: 1, register: 1},
		{name: "nonfinal finite use remains", maxUses: 2, register: 1, current: 1},
		{name: "unlimited remains", maxUses: 0, register: 1, current: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			tokenValue := createEnrollmentToken(t, service, ctx, now, test.maxUses)
			for range test.register {
				_, err := service.Register(ctx, connect.NewRequest(&cadestrov1.RegisterRequest{Token: tokenValue, Hostname: "agent", AgentVersion: "test", Csr: testEnrollmentCSR(t)}))
				require.NoError(t, err)
			}
			token, err := service.store.Queries().GetUsableRegistrationToken(ctx, db.GetUsableRegistrationTokenParams{ValueHash: tokenHash(tokenValue), ExpiresAt: now})
			if test.current == 0 {
				require.ErrorIs(t, err, sql.ErrNoRows)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.current, token.CurrentUses)
		})
	}
}

func TestDeletedEnrollmentTokenCannotRegister(t *testing.T) {
	service, ctx, now, _ := testService(t)
	service.ca = testEnrollmentCA(t, now)
	tokenValue := createEnrollmentToken(t, service, ctx, now, 0)
	token, err := service.store.Queries().GetUsableRegistrationToken(ctx, db.GetUsableRegistrationTokenParams{ValueHash: tokenHash(tokenValue), ExpiresAt: now})
	require.NoError(t, err)
	_, err = service.DeleteToken(ctx, connect.NewRequest(&cadestrov1.DeleteTokenRequest{Id: &cadestrov1.RegistrationTokenId{Value: token.ID}}))
	require.NoError(t, err)
	_, err = service.Register(ctx, connect.NewRequest(&cadestrov1.RegisterRequest{Token: tokenValue, Hostname: "agent", AgentVersion: "test", Csr: testEnrollmentCSR(t)}))
	require.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
}

func TestFinalUseDeletionUsesDatabaseState(t *testing.T) {
	service, ctx, now, _ := testService(t)
	tokenValue := createEnrollmentToken(t, service, ctx, now, 2)
	snapshot, err := service.store.Queries().GetUsableRegistrationToken(ctx, db.GetUsableRegistrationTokenParams{ValueHash: tokenHash(tokenValue), ExpiresAt: now})
	require.NoError(t, err)
	for range 2 {
		err = service.store.Transaction(ctx, func(queries *db.Queries) error {
			_, err := consumeRegistrationToken(ctx, queries, snapshot, now)
			return err
		})
		require.NoError(t, err)
	}
	_, err = service.store.Queries().GetRegistrationToken(ctx, snapshot.ID)
	require.ErrorIs(t, err, sql.ErrNoRows)
}
