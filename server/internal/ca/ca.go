package ca

import (
	"bytes"
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"os"
	"time"

	"github.com/manchtools/cadestro/server/internal/mtls"
)

var ErrInvalidCSR = errors.New("invalid certificate signing request")

func EnrollmentIdentityFromCSR(csrPEM []byte) ([]byte, error) {
	_, key, err := parseEnrollmentCSR(csrPEM)
	if err != nil {
		return nil, err
	}
	return append([]byte(nil), key...), nil
}

func parseEnrollmentCSR(csrPEM []byte) (*x509.CertificateRequest, ed25519.PublicKey, error) {
	block, rest := pem.Decode(csrPEM)
	if block == nil {
		return nil, nil, fmt.Errorf("%w: failed to decode CSR PEM", ErrInvalidCSR)
	}
	if block.Type != "CERTIFICATE REQUEST" && block.Type != "NEW CERTIFICATE REQUEST" {
		return nil, nil, fmt.Errorf("%w: unexpected PEM block type %q", ErrInvalidCSR, block.Type)
	}
	if len(bytes.TrimSpace(rest)) != 0 {
		return nil, nil, fmt.Errorf("%w: trailing data after CSR PEM", ErrInvalidCSR)
	}
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: parse CSR: %v", ErrInvalidCSR, err)
	}
	if err := csr.CheckSignature(); err != nil {
		return nil, nil, fmt.Errorf("%w: invalid CSR signature: %v", ErrInvalidCSR, err)
	}
	key, ok := csr.PublicKey.(ed25519.PublicKey)
	if !ok {
		return nil, nil, fmt.Errorf("%w: unsupported public key type %T: Ed25519 is required", ErrInvalidCSR, csr.PublicKey)
	}
	if len(csr.DNSNames) > 0 || len(csr.IPAddresses) > 0 || len(csr.EmailAddresses) > 0 || len(csr.URIs) > 0 {
		return nil, nil, fmt.Errorf("%w: CSR must not request subject alternative names", ErrInvalidCSR)
	}
	return csr, key, nil
}

type CA struct {
	cert         *x509.Certificate
	key          crypto.Signer
	validity     time.Duration
	activeCAPool *x509.CertPool
	now          func() time.Time
}

type Option func(*CA)

func WithClock(now func() time.Time) Option { return func(c *CA) { c.now = now } }

type Certificate struct {
	CertPEM     []byte
	KeyPEM      []byte
	Fingerprint string
	NotAfter    time.Time
}

func SerialFromCert(cert *x509.Certificate) (string, error) {
	if cert == nil || cert.SerialNumber == nil || cert.SerialNumber.Sign() < 0 {
		return "", errors.New("certificate serial is required")
	}
	return cert.SerialNumber.Text(16), nil
}

func SerialFromPEM(certPEM []byte) (string, error) {
	block, rest := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" || len(bytes.TrimSpace(rest)) != 0 {
		return "", errors.New("invalid certificate PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("parse certificate: %w", err)
	}
	return SerialFromCert(cert)
}

func New(certPath, keyPath string, validity time.Duration, opts ...Option) (*CA, error) {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, fmt.Errorf("read CA certificate: %w", err)
	}

	keyInfo, err := os.Stat(keyPath)
	if err != nil {
		return nil, fmt.Errorf("stat CA key: %w", err)
	}
	if keyInfo.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("CA private key file %q must not be group/world accessible (mode %#o)", keyPath, keyInfo.Mode().Perm())
	}

	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("read CA key: %w", err)
	}

	return NewFromPEM(certPEM, keyPEM, validity, opts...)
}

func NewFromPEM(certPEM, keyPEM []byte, validity time.Duration, opts ...Option) (*CA, error) {
	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil {
		return nil, fmt.Errorf("failed to decode CA certificate PEM")
	}

	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse CA certificate: %w", err)
	}

	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return nil, fmt.Errorf("failed to decode CA key PEM")
	}

	key, err := parsePrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse CA key: %w", err)
	}

	if _, ok := key.(ed25519.PrivateKey); !ok {
		return nil, fmt.Errorf("unsupported CA signing key type %T: Ed25519 is required", key)
	}
	certPublic, err := x509.MarshalPKIXPublicKey(cert.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("marshal CA certificate public key: %w", err)
	}
	keyPublic, err := x509.MarshalPKIXPublicKey(key.Public())
	if err != nil {
		return nil, fmt.Errorf("marshal CA private-key public key: %w", err)
	}
	if !bytes.Equal(certPublic, keyPublic) {
		return nil, fmt.Errorf("CA certificate and private key do not match")
	}

	pool := x509.NewCertPool()
	pool.AddCert(cert)

	c := &CA{
		cert:         cert,
		key:          key,
		validity:     validity,
		activeCAPool: pool,
		now:          time.Now,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

const (
	serverCertValidity = 45 * 24 * time.Hour
	clockSkewAllowance = time.Minute
)

func (ca *CA) IssueCertificateFromCSR(deviceID string, csrPEM []byte) (*Certificate, error) {
	return ca.issueFromCSR(deviceID, csrPEM, mtls.PeerClassAgent, ca.validity, nil)
}

func (ca *CA) IssueServerCertificateFromCSR(id string, csrPEM []byte, hostname string) (*Certificate, error) {
	var dnsNames []string
	if hostname != "" {
		dnsNames = []string{hostname}
	}
	return ca.issueFromCSR(id, csrPEM, mtls.PeerClassControl, serverCertValidity, dnsNames)
}

func (ca *CA) issueFromCSR(deviceID string, csrPEM []byte, class mtls.PeerClass, validity time.Duration, dnsNames []string) (*Certificate, error) {
	csr, _, err := parseEnrollmentCSR(csrPEM)
	if err != nil {
		return nil, err
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("generate serial number: %w", err)
	}

	now := ca.now()
	notAfter := now.Add(validity)

	peerURI, err := mtls.PeerClassURI(class)
	if err != nil {
		return nil, fmt.Errorf("build peer-class URI: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   deviceID,
			SerialNumber: deviceID,
			Organization: []string{"cadestro"},
		},
		NotBefore:             now.Add(-clockSkewAllowance),
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           certExtKeyUsage(len(dnsNames) > 0),
		BasicConstraintsValid: true,
		URIs:                  []*url.URL{peerURI},
		DNSNames:              dnsNames,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, ca.cert, csr.PublicKey, ca.key)
	if err != nil {
		return nil, fmt.Errorf("create certificate: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	fingerprint := sha256.Sum256(certDER)

	return &Certificate{
		CertPEM:     certPEM,
		KeyPEM:      nil,
		Fingerprint: hex.EncodeToString(fingerprint[:]),
		NotAfter:    notAfter,
	}, nil
}

func certExtKeyUsage(servesTLS bool) []x509.ExtKeyUsage {
	usage := []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	if servesTLS {
		usage = append(usage, x509.ExtKeyUsageServerAuth)
	}
	return usage
}

func (ca *CA) VerifyCertificate(certPEM []byte) (string, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return "", fmt.Errorf("failed to decode certificate PEM")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("parse certificate: %w", err)
	}

	if _, err := cert.Verify(x509.VerifyOptions{
		Roots:     ca.activeCAPool,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}); err != nil {
		return "", fmt.Errorf("certificate verification failed: %w", err)
	}
	return cert.Subject.CommonName, nil
}

func (ca *CA) TrustPool() *x509.CertPool {
	return ca.activeCAPool
}

func (ca *CA) CACertPEM() []byte {
	return pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: ca.cert.Raw,
	})
}

func parsePrivateKey(der []byte) (crypto.Signer, error) {
	if key, err := x509.ParsePKCS8PrivateKey(der); err == nil {
		if signer, ok := key.(crypto.Signer); ok {
			return signer, nil
		}
	}

	if key, err := x509.ParseECPrivateKey(der); err == nil {
		return key, nil
	}

	if key, err := x509.ParsePKCS1PrivateKey(der); err == nil {
		return key, nil
	}

	return nil, fmt.Errorf("unsupported private key format")
}

func FingerprintFromPEM(certPEM []byte) (string, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return "", fmt.Errorf("failed to decode certificate PEM")
	}

	fingerprint := sha256.Sum256(block.Bytes)
	return hex.EncodeToString(fingerprint[:]), nil
}

func NotAfterFromPEM(certPEM []byte) (time.Time, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return time.Time{}, fmt.Errorf("failed to decode certificate PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse certificate: %w", err)
	}
	return cert.NotAfter, nil
}

func FingerprintFromCert(cert *x509.Certificate) string {
	if cert == nil {
		return ""
	}
	fingerprint := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(fingerprint[:])
}

func DeviceIDFromPEM(certPEM []byte) (string, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return "", fmt.Errorf("failed to decode certificate PEM")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("parse certificate: %w", err)
	}

	return cert.Subject.CommonName, nil
}

func AssertCSRMatchesCert(cert *x509.Certificate, csrPEM []byte) error {
	if cert == nil {
		return errors.New("certificate is required")
	}
	block, rest := pem.Decode(csrPEM)
	if block == nil || len(bytes.TrimSpace(rest)) != 0 {
		return errors.New("invalid certificate signing request")
	}
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return fmt.Errorf("parse CSR: %w", err)
	}
	type equalKey interface{ Equal(crypto.PublicKey) bool }
	certKey, ok := cert.PublicKey.(equalKey)
	if !ok || !certKey.Equal(csr.PublicKey) {
		return errors.New("CSR public key does not match authenticated TLS peer")
	}
	return nil
}
