package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log/slog"
	"time"

	"github.com/manchtools/cadestro/agent/internal/credentials"
	sdk "github.com/manchtools/cadestro/contract"
	sdkcrypto "github.com/manchtools/cadestro/sdk/crypto"
)

func presentPendingCertificate(pending, fallbackActive bool) bool {
	return pending && !fallbackActive
}

func configureAgentMTLS(creds *credentials.Credentials, fallbackActive bool) (sdk.ClientOption, bool, bool, error) {
	usingPending := presentPendingCertificate(len(creds.PendingCertificate) > 0, fallbackActive)
	presentedCertificate := creds.Certificate
	if usingPending {
		presentedCertificate = creds.PendingCertificate
	}
	mtlsOpt, err := sdk.WithMTLSFromPEM(presentedCertificate, creds.PrivateKey, creds.CACert)
	if err != nil && usingPending {
		return nil, true, true, err
	}
	return mtlsOpt, usingPending, false, err
}

func fallbackAfterConnection(pending, presentedPending, connected bool) bool {
	if !pending || connected {
		return false
	}
	return presentedPending
}

func validateRenewalCertificate(certPEM, keyPEM, caPEM []byte, deviceID string) error {
	block, rest := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" || len(bytes.TrimSpace(rest)) != 0 {
		return fmt.Errorf("renewal returned malformed certificate PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("parse renewal certificate: %w", err)
	}
	if cert.Subject.CommonName != deviceID || cert.Subject.SerialNumber != deviceID {
		return fmt.Errorf("renewal certificate identity does not match device")
	}
	caBlock, caRest := pem.Decode(caPEM)
	if caBlock == nil || caBlock.Type != "CERTIFICATE" || len(bytes.TrimSpace(caRest)) != 0 {
		return fmt.Errorf("decode pinned CA certificate")
	}
	caCert, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil {
		return fmt.Errorf("parse pinned CA certificate: %w", err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(caCert)
	if _, err := cert.Verify(x509.VerifyOptions{Roots: roots, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}); err != nil {
		return fmt.Errorf("verify renewal certificate against pinned CA: %w", err)
	}
	if _, err := tls.X509KeyPair(certPEM, keyPEM); err != nil {
		return fmt.Errorf("renewal certificate does not match credential key: %w", err)
	}
	return nil
}

func certificateRenewalDue(cert *x509.Certificate, now time.Time) bool {
	return !now.Before(cert.NotBefore.Add(time.Duration(float64(cert.NotAfter.Sub(cert.NotBefore)) * .8)))
}

func applyRenewal(creds *credentials.Credentials, result *sdk.RenewCertificateResult) error {
	if len(result.Certificate) == 0 {
		return fmt.Errorf("renewal returned an empty certificate")
	}
	if err := validateRenewalCertificate(result.Certificate, creds.PrivateKey, creds.CACert, creds.DeviceID); err != nil {
		return err
	}
	creds.PendingCertificate = append([]byte(nil), result.Certificate...)
	return nil
}

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
	csr, err := sdkcrypto.GenerateCSRFromKey(hostname, creds.PrivateKey)
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
	if err := credStore.Save(ctx, creds); err != nil {
		return false, err
	}
	logger.Info("certificate renewal staged", "not_after", result.NotAfter)
	return true, nil
}
