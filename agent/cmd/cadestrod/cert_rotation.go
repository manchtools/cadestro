package main

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log/slog"
	"time"

	"github.com/manchtools/cadestro/agent/internal/credentials"
	sdk "github.com/manchtools/cadestro/contract"
	pmcrypto "github.com/manchtools/cadestro/sdk/crypto"
)

func presentPendingCertificate(pending, fallbackActive bool) bool {
	return pending && !fallbackActive
}

func fallbackAfterConnection(pending, presentedPending, connected bool) bool {
	if !pending || connected {
		return false
	}
	return presentedPending
}

func certificateRenewalDue(cert *x509.Certificate, now time.Time) bool {
	return !now.Before(cert.NotBefore.Add(time.Duration(float64(cert.NotAfter.Sub(cert.NotBefore)) * .8)))
}

// applyRenewal records B beside A. CA trust remains pinned at enrollment.
func applyRenewal(creds *credentials.Credentials, result *sdk.RenewCertificateResult) error {
	if len(result.Certificate) == 0 {
		return fmt.Errorf("renewal returned an empty certificate")
	}
	creds.PendingCertificate = append([]byte(nil), result.Certificate...)
	return nil
}

// renewCertificateIfDue is called by the existing connection/sync cadence;
// there is no second rotation goroutine or timer state to coordinate.
func renewCertificateIfDue(ctx context.Context, credStore *credentials.Store, creds *credentials.Credentials, hostname string, logger *slog.Logger, now func() time.Time, force bool) (bool, error) {
	block, _ := pem.Decode(creds.Certificate)
	if block == nil {
		return false, fmt.Errorf("decode active certificate")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return false, err
	}
	if !force && !certificateRenewalDue(cert, now()) {
		return false, nil
	}
	csr, err := pmcrypto.GenerateCSRFromKey(hostname, creds.PrivateKey)
	if err != nil {
		return false, err
	}
	opt, err := sdk.WithMTLSFromPEM(creds.Certificate, creds.PrivateKey, creds.CACert)
	if err != nil {
		return false, err
	}
	result, err := sdk.RenewCertificate(ctx, creds.AgentAddr, csr, opt)
	if err != nil {
		return false, err
	}
	if err := applyRenewal(creds, result); err != nil {
		return false, err
	}
	if err := credStore.Save(creds); err != nil {
		return false, err
	}
	logger.Info("certificate renewal staged", "not_after", result.NotAfter)
	return true, nil
}
