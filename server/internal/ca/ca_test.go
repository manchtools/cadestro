package ca_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manchtools/cadestro/server/internal/ca"
	"github.com/manchtools/cadestro/server/internal/mtls"
)

func generateTestCA(t *testing.T) (certPEM, keyPEM []byte) {
	t.Helper()

	caPublic, caKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName:   "Test CA",
			Organization: []string{"Test"},
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, caPublic, caKey)
	require.NoError(t, err)

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	keyDER, err := x509.MarshalPKCS8PrivateKey(caKey)
	require.NoError(t, err)
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})

	return certPEM, keyPEM
}

func generateCSR(t *testing.T, deviceID string) (csrPEM []byte, key ed25519.PrivateKey) {
	t.Helper()

	_, key, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	csrTemplate := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName: deviceID,
		},
	}

	csrDER, err := x509.CreateCertificateRequest(rand.Reader, csrTemplate, key)
	require.NoError(t, err)

	csrPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER})
	return csrPEM, key
}

func csrForKey(t *testing.T, deviceID string, key ed25519.PrivateKey) []byte {
	t.Helper()
	der, err := x509.CreateCertificateRequest(rand.Reader,
		&x509.CertificateRequest{Subject: pkix.Name{CommonName: deviceID}}, key)
	require.NoError(t, err)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der})
}

func TestAssertCSRMatchesCert(t *testing.T) {
	certPEM, keyPEM := generateTestCA(t)
	c, err := ca.NewFromPEM(certPEM, keyPEM, 24*time.Hour)
	require.NoError(t, err)

	_, deviceKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	issued, err := c.IssueCertificateFromCSR("device-001", csrForKey(t, "device-001", deviceKey))
	require.NoError(t, err)
	block, _ := pem.Decode(issued.CertPEM)
	peer, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)

	t.Run("matching key passes", func(t *testing.T) {
		require.NoError(t, ca.AssertCSRMatchesCert(peer, csrForKey(t, "device-001", deviceKey)))
	})
	t.Run("mismatched key rejected", func(t *testing.T) {
		_, other, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		err = ca.AssertCSRMatchesCert(peer, csrForKey(t, "device-001", other))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not match")
	})
	t.Run("malformed cert PEM rejected", func(t *testing.T) {
		assert.Error(t, ca.AssertCSRMatchesCert(nil, csrForKey(t, "device-001", deviceKey)))
	})
	t.Run("malformed CSR PEM rejected", func(t *testing.T) {
		assert.Error(t, ca.AssertCSRMatchesCert(peer, []byte("not a csr")))
	})
}

func TestSerialFromCert_CanonicalLowercaseHex(t *testing.T) {
	certPEM, keyPEM := generateTestCA(t)
	c, err := ca.NewFromPEM(certPEM, keyPEM, 24*time.Hour)
	require.NoError(t, err)
	csrPEM, _ := generateCSR(t, "device-001")
	issued, err := c.IssueCertificateFromCSR("device-001", csrPEM)
	require.NoError(t, err)
	block, _ := pem.Decode(issued.CertPEM)
	parsed, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)

	serial, err := ca.SerialFromCert(parsed)
	require.NoError(t, err)
	assert.Equal(t, strings.ToLower(serial), serial, "serial must already be lower-case")
	assert.Equal(t, parsed.SerialNumber.Text(16), serial)
}

func TestSerialFromCert_RejectsMissingOrNegativeSerial(t *testing.T) {
	_, err := ca.SerialFromCert(nil)
	assert.Error(t, err)

	_, err = ca.SerialFromCert(&x509.Certificate{})
	assert.Error(t, err, "a certificate with no serial number must be rejected")

	_, err = ca.SerialFromCert(&x509.Certificate{SerialNumber: big.NewInt(-1)})
	assert.Error(t, err, "a negative serial number must be rejected")
}

func TestSerialFromPEM_MatchesSerialFromCert(t *testing.T) {
	certPEM, keyPEM := generateTestCA(t)
	c, err := ca.NewFromPEM(certPEM, keyPEM, 24*time.Hour)
	require.NoError(t, err)
	csrPEM, _ := generateCSR(t, "device-001")
	issued, err := c.IssueCertificateFromCSR("device-001", csrPEM)
	require.NoError(t, err)
	block, _ := pem.Decode(issued.CertPEM)
	parsed, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)

	fromCert, err := ca.SerialFromCert(parsed)
	require.NoError(t, err)
	fromPEM, err := ca.SerialFromPEM(issued.CertPEM)
	require.NoError(t, err)
	assert.Equal(t, fromCert, fromPEM)
}

func TestSerialFromPEM_StrictDecoding(t *testing.T) {
	certPEM, keyPEM := generateTestCA(t)
	c, err := ca.NewFromPEM(certPEM, keyPEM, 24*time.Hour)
	require.NoError(t, err)
	csrPEM, _ := generateCSR(t, "device-001")
	issued, err := c.IssueCertificateFromCSR("device-001", csrPEM)
	require.NoError(t, err)

	t.Run("wrong PEM block type rejected", func(t *testing.T) {
		block, _ := pem.Decode(issued.CertPEM)
		require.NotNil(t, block)
		block.Type = "CERTIFICATE REQUEST"
		_, err := ca.SerialFromPEM(pem.EncodeToMemory(block))
		assert.Error(t, err)
	})
	t.Run("trailing data after PEM rejected", func(t *testing.T) {
		_, err := ca.SerialFromPEM(append(append([]byte(nil), issued.CertPEM...), []byte("trailing")...))
		assert.Error(t, err)
	})
	t.Run("malformed PEM rejected", func(t *testing.T) {
		_, err := ca.SerialFromPEM([]byte("not a certificate"))
		assert.Error(t, err)
	})
}

func TestNewFromPEM(t *testing.T) {
	certPEM, keyPEM := generateTestCA(t)

	c, err := ca.NewFromPEM(certPEM, keyPEM, 24*time.Hour)
	require.NoError(t, err)
	assert.NotNil(t, c)
}

func TestNewFromPEM_InvalidCert(t *testing.T) {
	_, keyPEM := generateTestCA(t)

	_, err := ca.NewFromPEM([]byte("not a cert"), keyPEM, 24*time.Hour)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "decode CA certificate PEM")
}

func TestNewFromPEM_InvalidKey(t *testing.T) {
	certPEM, _ := generateTestCA(t)

	_, err := ca.NewFromPEM(certPEM, []byte("not a key"), 24*time.Hour)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "decode CA key PEM")
}

func TestIssueCertificateFromCSR_Success(t *testing.T) {
	certPEM, keyPEM := generateTestCA(t)
	c, err := ca.NewFromPEM(certPEM, keyPEM, 24*time.Hour)
	require.NoError(t, err)

	csrPEM, _ := generateCSR(t, "device-001")

	cert, err := c.IssueCertificateFromCSR("device-001", csrPEM)
	require.NoError(t, err)
	assert.NotEmpty(t, cert.CertPEM)
	assert.True(t, cert.NotAfter.After(time.Now()))
}

func TestIssueCertificateFromCSR_IdentityComesFromServerNotCSR(t *testing.T) {
	certPEM, keyPEM := generateTestCA(t)
	c, err := ca.NewFromPEM(certPEM, keyPEM, 24*time.Hour)
	require.NoError(t, err)

	const csrChosenID = "attacker-chosen-id"
	const serverAuthoritativeID = "real-device-7"
	csrPEM, _ := generateCSR(t, csrChosenID)

	cert, err := c.IssueCertificateFromCSR(serverAuthoritativeID, csrPEM)
	require.NoError(t, err)

	block, _ := pem.Decode(cert.CertPEM)
	require.NotNil(t, block)
	parsed, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	roots := x509.NewCertPool()
	require.True(t, roots.AppendCertsFromPEM(certPEM))
	_, err = parsed.Verify(x509.VerifyOptions{Roots: roots, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}})
	require.NoError(t, err)
	assert.Equal(t, serverAuthoritativeID, parsed.Subject.CommonName, "CN must be the server id, not the CSR CN")
	assert.Equal(t, serverAuthoritativeID, parsed.Subject.SerialNumber, "Subject.SerialNumber must be the server id")
	assert.NotEqual(t, csrChosenID, parsed.Subject.CommonName, "the attacker-controlled CSR CN must never become the cert identity")
}

func generateCSRWithSAN(t *testing.T, deviceID string, modify func(*x509.CertificateRequest)) []byte {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.CertificateRequest{Subject: pkix.Name{CommonName: deviceID}}
	modify(tmpl)
	der, err := x509.CreateCertificateRequest(rand.Reader, tmpl, key)
	require.NoError(t, err)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der})
}

func TestIssueCertificateFromCSR_RejectsSAN(t *testing.T) {
	certPEM, keyPEM := generateTestCA(t)
	c, err := ca.NewFromPEM(certPEM, keyPEM, 24*time.Hour)
	require.NoError(t, err)

	mustURL := func(s string) *url.URL {
		u, perr := url.Parse(s)
		require.NoError(t, perr)
		return u
	}
	cases := map[string]func(*x509.CertificateRequest){
		"spiffe gateway URI": func(r *x509.CertificateRequest) { r.URIs = []*url.URL{mustURL("spiffe://cadestro/gateway")} },
		"spiffe control URI": func(r *x509.CertificateRequest) { r.URIs = []*url.URL{mustURL("spiffe://cadestro/control")} },
		"dns name":           func(r *x509.CertificateRequest) { r.DNSNames = []string{"control-server.example.com"} },
		"ip address":         func(r *x509.CertificateRequest) { r.IPAddresses = []net.IP{net.ParseIP("10.0.0.1")} },
		"email":              func(r *x509.CertificateRequest) { r.EmailAddresses = []string{"x@y"} },
	}
	for name, modify := range cases {
		t.Run(name, func(t *testing.T) {
			csrPEM := generateCSRWithSAN(t, "device-001", modify)
			cert, err := c.IssueCertificateFromCSR("device-001", csrPEM)
			require.Error(t, err, "a CSR with a SAN must be rejected")
			assert.Contains(t, err.Error(), "must not request subject alternative names")
			assert.Nil(t, cert, "no certificate must be issued for a SAN-bearing CSR")
		})
	}

	csrPEM, _ := generateCSR(t, "device-001")
	cert, err := c.IssueCertificateFromCSR("device-001", csrPEM)
	require.NoError(t, err)
	require.NotNil(t, cert)
}

func TestIssueCertificateFromCSR_StampsExactlyAgentPeerClass(t *testing.T) {
	certPEM, keyPEM := generateTestCA(t)
	c, err := ca.NewFromPEM(certPEM, keyPEM, 24*time.Hour)
	require.NoError(t, err)

	csrPEM, _ := generateCSR(t, "device-001")
	cert, err := c.IssueCertificateFromCSR("device-001", csrPEM)
	require.NoError(t, err)

	block, _ := pem.Decode(cert.CertPEM)
	require.NotNil(t, block)
	parsed, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)

	require.Len(t, parsed.URIs, 1, "an issued agent cert must carry exactly one URI SAN")
	assert.Equal(t, "spiffe://cadestro/agent", parsed.URIs[0].String())

	class, err := mtls.PeerClassFromCert(parsed)
	require.NoError(t, err)
	assert.Equal(t, mtls.PeerClassAgent, class, "an issued cert must always be the agent peer class")
}

func TestIssueCertificateFromCSR_ValidityWindowFromClock(t *testing.T) {
	certPEM, keyPEM := generateTestCA(t)
	fixed := time.Date(2020, 6, 1, 12, 0, 0, 0, time.UTC)
	const validity = 24 * time.Hour
	c, err := ca.NewFromPEM(certPEM, keyPEM, validity, ca.WithClock(func() time.Time { return fixed }))
	require.NoError(t, err)

	csrPEM, _ := generateCSR(t, "device-001")
	cert, err := c.IssueCertificateFromCSR("device-001", csrPEM)
	require.NoError(t, err)

	assert.True(t, cert.NotAfter.Equal(fixed.Add(validity)),
		"NotAfter must be clock+validity; got %s want %s", cert.NotAfter, fixed.Add(validity))
	assert.True(t, cert.NotAfter.Before(time.Now()),
		"a cert issued under a past clock must already be expired, proving the window is not from the wall clock")

	block, _ := pem.Decode(cert.CertPEM)
	require.NotNil(t, block)
	parsed, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	assert.True(t, parsed.NotBefore.Equal(fixed.Add(-1*time.Minute)),
		"NotBefore must be clock-1m (skew); got %s want %s", parsed.NotBefore, fixed.Add(-1*time.Minute))
	assert.True(t, parsed.NotAfter.Equal(fixed.Add(validity)),
		"encoded NotAfter must be clock+validity; got %s want %s", parsed.NotAfter, fixed.Add(validity))
}

func TestIssueCertificateFromCSR_InvalidCSR(t *testing.T) {
	certPEM, keyPEM := generateTestCA(t)
	c, err := ca.NewFromPEM(certPEM, keyPEM, 24*time.Hour)
	require.NoError(t, err)

	_, err = c.IssueCertificateFromCSR("device-001", []byte("not a csr"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "decode CSR PEM")
}

func TestEnrollmentIdentityFromCSRRejectsTrailingData(t *testing.T) {
	csr, _ := generateCSR(t, "device-001")
	second, _ := generateCSR(t, "device-002")
	for name, suffix := range map[string][]byte{
		"text":   []byte("trailing"),
		"second": append([]byte("\n"), second...),
	} {
		t.Run(name, func(t *testing.T) {
			input := append(append([]byte(nil), csr...), suffix...)
			_, err := ca.EnrollmentIdentityFromCSR(input)
			require.ErrorIs(t, err, ca.ErrInvalidCSR)
			assert.Contains(t, err.Error(), "trailing data")
		})
	}
}

func TestEnrollmentIdentityFromCSRRejectsWrongPEMType(t *testing.T) {
	csr, _ := generateCSR(t, "device-001")
	block, _ := pem.Decode(csr)
	require.NotNil(t, block)
	block.Type = "CERTIFICATE"
	_, err := ca.EnrollmentIdentityFromCSR(pem.EncodeToMemory(block))
	require.ErrorIs(t, err, ca.ErrInvalidCSR)
	assert.Contains(t, err.Error(), "unexpected PEM block type")
}

func TestIssueCertificateFromCSR_ForgedSignatureRejected(t *testing.T) {
	certPEM, keyPEM := generateTestCA(t)
	c, err := ca.NewFromPEM(certPEM, keyPEM, 24*time.Hour)
	require.NoError(t, err)

	csrPEM, _ := generateCSR(t, "device-001")
	block, _ := pem.Decode(csrPEM)
	require.NotNil(t, block)
	der := append([]byte(nil), block.Bytes...)
	der[len(der)-1] ^= 0xFF
	forged := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der})

	_, perr := x509.ParseCertificateRequest(der)
	require.NoError(t, perr, "forged CSR must remain structurally parseable")

	_, err = c.IssueCertificateFromCSR("device-001", forged)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid CSR signature")
}

func TestIssueCertificateFromCSR_AgentIsClientAuthOnly(t *testing.T) {
	certPEM, keyPEM := generateTestCA(t)
	c, err := ca.NewFromPEM(certPEM, keyPEM, 24*time.Hour)
	require.NoError(t, err)
	csrPEM, _ := generateCSR(t, "device-001")
	cert, err := c.IssueCertificateFromCSR("device-001", csrPEM)
	require.NoError(t, err)
	block, _ := pem.Decode(cert.CertPEM)
	require.NotNil(t, block)
	parsed, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	assert.Equal(t, []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, parsed.ExtKeyUsage,
		"an agent cert must be client-auth only")
}

func TestCACertPEM(t *testing.T) {
	certPEM, keyPEM := generateTestCA(t)
	c, err := ca.NewFromPEM(certPEM, keyPEM, 24*time.Hour)
	require.NoError(t, err)

	caCert := c.CACertPEM()
	assert.NotEmpty(t, caCert)

	block, _ := pem.Decode(caCert)
	require.NotNil(t, block)
	assert.Equal(t, "CERTIFICATE", block.Type)
}

func TestTrustPool(t *testing.T) {
	certPEM, keyPEM := generateTestCA(t)
	c, err := ca.NewFromPEM(certPEM, keyPEM, 24*time.Hour)
	require.NoError(t, err)

	pool := c.TrustPool()
	assert.NotNil(t, pool)
}
