package ca

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
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
	key          ed25519.PrivateKey
	validity     time.Duration
	activeCAPool *x509.CertPool
	now          func() time.Time
}

type Option func(*CA)

func WithClock(now func() time.Time) Option { return func(c *CA) { c.now = now } }

type Certificate struct {
	CertPEM  []byte
	NotAfter time.Time
}

func SerialFromCert(cert *x509.Certificate) (string, error) {
	if cert == nil || cert.SerialNumber == nil || cert.SerialNumber.Sign() < 0 {
		return "", errors.New("certificate serial is required")
	}
	return cert.SerialNumber.Text(16), nil
}

func SerialFromPEM(certPEM []byte) (string, error) {
	cert, err := parseCertificatePEM(certPEM)
	if err != nil {
		return "", err
	}
	return SerialFromCert(cert)
}

func parseCertificatePEM(certPEM []byte) (*x509.Certificate, error) {
	block, rest := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" || len(bytes.TrimSpace(rest)) != 0 {
		return nil, errors.New("invalid certificate PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse certificate: %w", err)
	}
	return cert, nil
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

	parsedKey, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse CA key: %w", err)
	}
	key, ok := parsedKey.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("unsupported CA signing key type %T: Ed25519 is required", parsedKey)
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

const clockSkewAllowance = time.Minute

func (ca *CA) IssueCertificateFromCSR(deviceID string, csrPEM []byte) (*Certificate, error) {
	csr, _, err := parseEnrollmentCSR(csrPEM)
	if err != nil {
		return nil, err
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("generate serial number: %w", err)
	}

	now := ca.now()
	notAfter := now.Add(ca.validity)

	peerURI, err := mtls.PeerClassURI(mtls.PeerClassAgent)
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
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		URIs:                  []*url.URL{peerURI},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, ca.cert, csr.PublicKey, ca.key)
	if err != nil {
		return nil, fmt.Errorf("create certificate: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	return &Certificate{
		CertPEM:  certPEM,
		NotAfter: notAfter,
	}, nil
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
	certKey, certOK := cert.PublicKey.(ed25519.PublicKey)
	csrKey, csrOK := csr.PublicKey.(ed25519.PublicKey)
	if !certOK || !csrOK || !bytes.Equal(certKey, csrKey) {
		return errors.New("CSR public key does not match authenticated TLS peer")
	}
	return nil
}
