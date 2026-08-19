package main

import (
	"crypto/x509"
	"testing"
	"time"

	"github.com/manchtools/cadestro/agent/internal/credentials"
	sdk "github.com/manchtools/cadestro/contract"
)

func TestCertificateRenewalDue_Computation(t *testing.T) {
	nb := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	na := nb.Add(100 * 24 * time.Hour) // 100-day cert → renew at +80d
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

func TestApplyRenewal_StagesPendingCertificate(t *testing.T) {
	creds := &credentials.Credentials{Certificate: []byte("old-cert")}
	if err := applyRenewal(creds, &sdk.RenewCertificateResult{Certificate: []byte("new-cert")}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(creds.Certificate) != "old-cert" || string(creds.PendingCertificate) != "new-cert" {
		t.Fatal("renewal must preserve A and stage B")
	}
}
