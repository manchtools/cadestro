package main

import (
	"bytes"
	"context"
	"crypto/x509"
	"testing"
	"time"

	"github.com/manchtools/cadestro/agent/internal/credentials"
	sdk "github.com/manchtools/cadestro/contract"
	"github.com/manchtools/cadestro/sdk/cryptotest"
)

func TestCertificateRenewalDue_Computation(t *testing.T) {
	nb := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	na := nb.Add(100 * 24 * time.Hour)
	cert := &x509.Certificate{NotBefore: nb, NotAfter: na}

	if certificateRenewalDue(cert, nb) {
		t.Error("certificate is due at issuance")
	}
	if certificateRenewalDue(cert, nb.Add(50*24*time.Hour)) {
		t.Error("certificate is due before the 80% point")
	}
	if !certificateRenewalDue(cert, nb.Add(80*24*time.Hour)) {
		t.Error("certificate is not due at the 80% point")
	}
	if !certificateRenewalDue(cert, na.Add(time.Hour)) {
		t.Error("expired certificate is not due")
	}
}

type blockingWelcomeWaiter struct{}

func (blockingWelcomeWaiter) WaitConnected(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestWaitForWelcomeIsBounded(t *testing.T) {
	ctx, cancelSession := context.WithCancel(context.Background())
	defer cancelSession()
	started := time.Now()
	err := waitForWelcome(ctx, cancelSession, blockingWelcomeWaiter{}.WaitConnected, 5*time.Millisecond)
	if err == nil {
		t.Fatal("missing Welcome must return an error")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("missing Welcome waited too long: %v", elapsed)
	}
	select {
	case <-ctx.Done():
	default:
		t.Fatal("missing Welcome must cancel the stream session")
	}
}

func TestPendingCredentialSelectionAlternatesAfterFailure(t *testing.T) {
	for _, test := range []struct {
		name, initialFallback string
		presented, connected  bool
		wantFallback          bool
	}{
		{name: "pending B fails then A", presented: true, wantFallback: true},
		{name: "fallback A fails then B", initialFallback: "fallback", presented: false, wantFallback: false},
		{name: "B connected promotes", presented: true, connected: true, wantFallback: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			pending := true
			if test.initialFallback == "fallback" && presentPendingCertificate(pending, true) {
				t.Fatal("fallback A must be selected after a failed B attempt")
			}
			if got := fallbackAfterConnection(pending, test.presented, test.connected); got != test.wantFallback {
				t.Fatalf("fallbackAfterConnection = %v, want %v", got, test.wantFallback)
			}
		})
	}
}

func TestCorruptPendingCredentialFallsBackBeforeDial(t *testing.T) {
	creds := &credentials.Credentials{
		Certificate:        []byte("active certificate"),
		PendingCertificate: []byte("corrupt pending certificate"),
	}

	opt, usingPending, fallback, err := configureAgentMTLS(creds, false)
	if opt != nil {
		t.Fatal("corrupt pending certificate must not produce an mTLS option")
	}
	if !usingPending {
		t.Fatal("pending certificate must be selected before its configuration fails")
	}
	if !fallback {
		t.Fatal("corrupt pending certificate must select active-credential fallback")
	}
	if err == nil {
		t.Fatal("corrupt pending certificate must report its configuration error")
	}
}

func TestApplyRenewal_StagesPendingCertificate(t *testing.T) {
	caPEM, caKey, caCert := cryptotest.GenCA(t, "test-ca")
	certPEM, keyPEM := cryptotest.GenLeaf(t, caCert, caKey, "dev-1", false)
	creds := &credentials.Credentials{DeviceID: "dev-1", Certificate: certPEM, PendingCertificate: []byte("corrupt pending"), PrivateKey: keyPEM, CACert: caPEM}
	if err := applyRenewal(creds, &sdk.RenewCertificateResult{Certificate: certPEM}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(creds.Certificate) != string(certPEM) || string(creds.PendingCertificate) != string(certPEM) {
		t.Fatal("renewal must preserve A and stage B")
	}
}

func TestApplyRenewalRejectsTrailingCertificateData(t *testing.T) {
	caPEM, caKey, caCert := cryptotest.GenCA(t, "test-ca")
	activePEM, activeKey := cryptotest.GenLeaf(t, caCert, caKey, "dev-1", false)
	creds := &credentials.Credentials{DeviceID: "dev-1", Certificate: activePEM, PendingCertificate: []byte("still usable"), PrivateKey: activeKey, CACert: caPEM}
	trailing := append(append([]byte(nil), activePEM...), []byte("trailing")...)
	if err := applyRenewal(creds, &sdk.RenewCertificateResult{Certificate: trailing}); err == nil {
		t.Fatal("trailing certificate data must be rejected")
	}
	if !bytes.Equal(creds.PendingCertificate, []byte("still usable")) {
		t.Fatal("rejected renewal must preserve the last usable pending certificate")
	}
}

func TestApplyRenewalRejectsCertificateWithWrongKey(t *testing.T) {
	caPEM, caKey, caCert := cryptotest.GenCA(t, "test-ca")
	activePEM, activeKey := cryptotest.GenLeaf(t, caCert, caKey, "dev-1", false)
	otherPEM, _ := cryptotest.GenLeaf(t, caCert, caKey, "dev-1", false)
	creds := &credentials.Credentials{DeviceID: "dev-1", Certificate: activePEM, PrivateKey: activeKey, CACert: caPEM}
	if err := applyRenewal(creds, &sdk.RenewCertificateResult{Certificate: otherPEM}); err == nil {
		t.Fatal("renewal certificate with a different public key must be rejected")
	}
	if len(creds.PendingCertificate) != 0 {
		t.Fatal("rejected renewal must not overwrite pending credentials")
	}
}
