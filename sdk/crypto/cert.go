package crypto

import (
	"bytes"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
)

// CAFingerprintFromPEM returns the lowercase-hex SHA-256 of the
// certificate's decoded DER bytes.
func CAFingerprintFromPEM(certPEM []byte) (string, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return "", fmt.Errorf("failed to decode certificate PEM")
	}
	sum := sha256.Sum256(block.Bytes)
	return hex.EncodeToString(sum[:]), nil
}

// VerifyCAContinuity reports whether newCAPEM is an acceptable rotation
// of oldCAPEM: it must be byte-identical to the enrolled CA, OR be a CA
// that is cross-signed by (chains to) the enrolled CA. An unrelated CA
// is refused — that is the trust-anchor swap an attacker (or a
// compromised/MITM'd control origin) would attempt at certificate
// renewal. A hard CA swap (a new self-signed root not cross-signed by
// the old one) is intentionally refused too: it requires re-enrollment,
// not silent adoption over the renewal channel.
func VerifyCAContinuity(oldCAPEM, newCAPEM []byte) error {
	if len(newCAPEM) == 0 {
		return fmt.Errorf("new CA is empty")
	}
	oldBlock, _ := pem.Decode(oldCAPEM)
	if oldBlock == nil {
		return fmt.Errorf("enrolled CA: failed to decode PEM")
	}
	newBlock, _ := pem.Decode(newCAPEM)
	if newBlock == nil {
		return fmt.Errorf("new CA: failed to decode PEM")
	}

	// The overwhelmingly common case — renewal returns the same CA.
	if bytes.Equal(oldBlock.Bytes, newBlock.Bytes) {
		return nil
	}

	oldCert, err := x509.ParseCertificate(oldBlock.Bytes)
	if err != nil {
		return fmt.Errorf("enrolled CA: %w", err)
	}
	newCert, err := x509.ParseCertificate(newBlock.Bytes)
	if err != nil {
		return fmt.Errorf("new CA: %w", err)
	}

	// Continuity: the new CA must be signed by the enrolled CA.
	// CheckSignatureFrom verifies the signature AND that oldCert is a CA
	// permitted to sign certificates (IsCA + BasicConstraints + CertSign
	// key usage), so an unrelated self-signed CA fails here.
	if err := newCert.CheckSignatureFrom(oldCert); err != nil {
		return fmt.Errorf("new CA does not chain to the enrolled CA: %w", err)
	}
	return nil
}
