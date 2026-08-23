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

// generateTestCA creates a self-signed CA cert and key for testing.
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

// generateCSR creates a CSR PEM for a given device ID.
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

// csrForKey builds a CSR PEM for deviceID signed by the given key. Unlike
// generateCSR it lets the caller reuse a key, which is what renewal does.
func csrForKey(t *testing.T, deviceID string, key ed25519.PrivateKey) []byte {
	t.Helper()
	der, err := x509.CreateCertificateRequest(rand.Reader,
		&x509.CertificateRequest{Subject: pkix.Name{CommonName: deviceID}}, key)
	require.NoError(t, err)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der})
}

// TestAssertCSRMatchesCert covers the renewal proof-of-possession helper
// (#361): a renewal CSR is accepted only when its public key equals the current
// certificate's.
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
	assert.Nil(t, cert.KeyPEM, "private key should stay on agent")
	assert.NotEmpty(t, cert.Fingerprint)
	assert.True(t, cert.NotAfter.After(time.Now()))
}

// TestIssueCertificateFromCSR_IdentityComesFromServerNotCSR pins the
// anti-impersonation contract: the issued identity (CN + Subject.SerialNumber)
// is taken from the SERVER-supplied deviceID, never the attacker-controlled CSR
// Subject. The CSR CN is set to a DIFFERENT value than the server id so the test
// cannot pass by coincidence — the Success test above used the same string for
// both, which could not distinguish "id from server" from "id from CSR".
func TestIssueCertificateFromCSR_IdentityComesFromServerNotCSR(t *testing.T) {
	certPEM, keyPEM := generateTestCA(t)
	c, err := ca.NewFromPEM(certPEM, keyPEM, 24*time.Hour)
	require.NoError(t, err)

	const csrChosenID = "attacker-chosen-id"      // whatever the agent put in the CSR
	const serverAuthoritativeID = "real-device-7" // the server's own authoritative id
	csrPEM, _ := generateCSR(t, csrChosenID)

	cert, err := c.IssueCertificateFromCSR(serverAuthoritativeID, csrPEM)
	require.NoError(t, err)

	got, err := c.VerifyCertificate(cert.CertPEM)
	require.NoError(t, err)
	assert.Equal(t, serverAuthoritativeID, got, "VerifyCertificate must report the SERVER id")
	assert.NotEqual(t, csrChosenID, got)

	gotPEM, err := ca.DeviceIDFromPEM(cert.CertPEM)
	require.NoError(t, err)
	assert.Equal(t, serverAuthoritativeID, gotPEM)

	// Parse the issued cert: BOTH identity fields must be the server id.
	block, _ := pem.Decode(cert.CertPEM)
	require.NotNil(t, block)
	parsed, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	assert.Equal(t, serverAuthoritativeID, parsed.Subject.CommonName, "CN must be the server id, not the CSR CN")
	assert.Equal(t, serverAuthoritativeID, parsed.Subject.SerialNumber, "Subject.SerialNumber must be the server id")
	assert.NotEqual(t, csrChosenID, parsed.Subject.CommonName, "the attacker-controlled CSR CN must never become the cert identity")
}

// generateCSRWithSAN builds a CSR for deviceID after letting the caller stamp a
// subject-alternative-name onto the template — used to prove the CA rejects any
// caller-supplied SAN (which would otherwise let an enrolling agent mint a
// non-agent peer-class identity).
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

// TestIssueCertificateFromCSR_RejectsSAN pins WS14 #1: the CA must reject ANY
// CSR carrying a SAN. "Wrong" cases are sourced from intent — identities an
// enrolling agent must never be able to mint (a gateway/control peer class, a
// server hostname/IP) — not from the validation rule. The load-bearing one is
// the spiffe gateway URI: without the SAN rejection an agent could request a
// gateway peer class and reach the InternalService.
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

	// Correct/absent: a plain CSR (no SAN) still issues.
	csrPEM, _ := generateCSR(t, "device-001")
	cert, err := c.IssueCertificateFromCSR("device-001", csrPEM)
	require.NoError(t, err)
	require.NotNil(t, cert)
}

// TestIssueCertificateFromCSR_StampsExactlyAgentPeerClass pins WS14 #1's
// positive half: an issued cert carries EXACTLY one URI SAN, the agent peer
// class — so an enrolling agent can never obtain a non-agent class, even via a
// CN/name collision.
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

// TestIssueCertificateFromCSR_ValidityWindowFromClock pins that the
// issued certificate's validity window derives from the injected clock,
// not the wall clock: NotBefore = clock - 1m (skew allowance) and
// NotAfter = clock + validity. Using a clock fixed in the PAST proves the
// window cannot have come from time.Now() — the cert would be expired
// today, which a wall-clock implementation could never produce.
func TestIssueCertificateFromCSR_ValidityWindowFromClock(t *testing.T) {
	certPEM, keyPEM := generateTestCA(t)
	fixed := time.Date(2020, 6, 1, 12, 0, 0, 0, time.UTC)
	const validity = 24 * time.Hour
	c, err := ca.NewFromPEM(certPEM, keyPEM, validity, ca.WithClock(func() time.Time { return fixed }))
	require.NoError(t, err)

	csrPEM, _ := generateCSR(t, "device-001")
	cert, err := c.IssueCertificateFromCSR("device-001", csrPEM)
	require.NoError(t, err)

	// NotAfter is exposed on the Certificate struct at full precision.
	assert.True(t, cert.NotAfter.Equal(fixed.Add(validity)),
		"NotAfter must be clock+validity; got %s want %s", cert.NotAfter, fixed.Add(validity))
	assert.True(t, cert.NotAfter.Before(time.Now()),
		"a cert issued under a past clock must already be expired, proving the window is not from the wall clock")

	// NotBefore lives on the encoded cert; ASN.1 truncates to the second,
	// which is lossless here (fixed has zero sub-second component).
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

// TestIssueCertificateFromCSR_ForgedSignatureRejected pins the csr.CheckSignature
// gate: a CSR whose ASN.1 structure is valid but whose signature does NOT verify
// (tampered in transit, or minted by someone who does not hold the private key)
// must be refused. The InvalidCSR test above only feeds garbage PEM; this covers
// the structurally-valid-but-forged-signature branch — an uncovered edge that
// guards proof-of-possession at issuance.
func TestIssueCertificateFromCSR_ForgedSignatureRejected(t *testing.T) {
	certPEM, keyPEM := generateTestCA(t)
	c, err := ca.NewFromPEM(certPEM, keyPEM, 24*time.Hour)
	require.NoError(t, err)

	// A structurally valid CSR, then corrupt its signature: the last byte of the
	// DER lies in the signatureValue, so ParseCertificateRequest still succeeds
	// while CheckSignature must fail.
	csrPEM, _ := generateCSR(t, "device-001")
	block, _ := pem.Decode(csrPEM)
	require.NotNil(t, block)
	der := append([]byte(nil), block.Bytes...)
	der[len(der)-1] ^= 0xFF
	forged := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der})

	// Sanity: the forged CSR still parses, so we are exercising CheckSignature,
	// not the parse path the InvalidCSR test already covers.
	_, perr := x509.ParseCertificateRequest(der)
	require.NoError(t, perr, "forged CSR must remain structurally parseable")

	_, err = c.IssueCertificateFromCSR("device-001", forged)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid CSR signature")
}

// A control server certificate carries the server identity, the control peer
// class, its assigned DNS name, and a fixed validity distinct from agent certs.
func TestIssueServerCertificateFromCSR_StampsControlClassAndValidity(t *testing.T) {
	certPEM, keyPEM := generateTestCA(t)
	fixed := time.Date(2020, 6, 1, 12, 0, 0, 0, time.UTC)
	// Agent validity is deliberately NOT 45d so the assertion below distinguishes
	// the control-server validity from the CA default.
	c, err := ca.NewFromPEM(certPEM, keyPEM, 24*time.Hour, ca.WithClock(func() time.Time { return fixed }))
	require.NoError(t, err)

	const controlID = "control-01JABCDEF" // server id; CSR CN is different on purpose
	const controlHost = "control.example.com"
	csrPEM := generateCSRWithSAN(t, "csr-chosen-id", func(*x509.CertificateRequest) {})

	cert, err := c.IssueServerCertificateFromCSR(controlID, csrPEM, controlHost)
	require.NoError(t, err)

	block, _ := pem.Decode(cert.CertPEM)
	require.NotNil(t, block)
	parsed, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)

	assert.Equal(t, controlID, parsed.Subject.CommonName)
	assert.Equal(t, controlID, parsed.Subject.SerialNumber)

	require.Len(t, parsed.URIs, 1, "a control cert must carry exactly one URI SAN")
	class, err := mtls.PeerClassFromCert(parsed)
	require.NoError(t, err)
	assert.Equal(t, mtls.PeerClassControl, class, "a server cert must carry the control peer class, never agent")

	const want45d = 45 * 24 * time.Hour
	assert.True(t, cert.NotAfter.Equal(fixed.Add(want45d)),
		"control NotAfter must be clock+45d; got %s want %s", cert.NotAfter, fixed.Add(want45d))

	// The helper can issue the control certificate used by mTLS integration tests.
	assert.Contains(t, parsed.ExtKeyUsage, x509.ExtKeyUsageClientAuth)
	assert.Contains(t, parsed.ExtKeyUsage, x509.ExtKeyUsageServerAuth,
		"a control cert must be usable as a TLS server cert")

	// The hostname must be stamped as a DNS SAN (server-chosen), so the agent's
	// standard TLS verification can match the gateway's public name.
	assert.Equal(t, []string{controlHost}, parsed.DNSNames,
		"the control hostname must be stamped as a DNS SAN")
}

// TestIssueCertificateFromCSR_AgentIsClientAuthOnly pins that agent certs do NOT
// gain ServerAuth. An agent is an mTLS client and never a server, so granting it would be
// unnecessary authority.
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

// The server-certificate path rejects caller-chosen SANs.
func TestIssueServerCertificateFromCSR_RejectsSAN(t *testing.T) {
	certPEM, keyPEM := generateTestCA(t)
	c, err := ca.NewFromPEM(certPEM, keyPEM, 24*time.Hour)
	require.NoError(t, err)

	mustURL := func(s string) *url.URL {
		u, perr := url.Parse(s)
		require.NoError(t, perr)
		return u
	}
	csrPEM := generateCSRWithSAN(t, "gw-1", func(r *x509.CertificateRequest) {
		r.URIs = []*url.URL{mustURL("spiffe://cadestro/control")}
	})
	cert, err := c.IssueServerCertificateFromCSR("gw-1", csrPEM, "gw-1.example.com")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must not request subject alternative names")
	assert.Nil(t, cert)
}

func TestVerifyCertificate_Success(t *testing.T) {
	certPEM, keyPEM := generateTestCA(t)
	c, err := ca.NewFromPEM(certPEM, keyPEM, 24*time.Hour)
	require.NoError(t, err)

	csrPEM, _ := generateCSR(t, "device-001")
	issued, err := c.IssueCertificateFromCSR("device-001", csrPEM)
	require.NoError(t, err)

	deviceID, err := c.VerifyCertificate(issued.CertPEM)
	require.NoError(t, err)
	assert.Equal(t, "device-001", deviceID)
}

func TestVerifyCertificate_WrongCA(t *testing.T) {
	certPEM1, keyPEM1 := generateTestCA(t)
	c1, err := ca.NewFromPEM(certPEM1, keyPEM1, 24*time.Hour)
	require.NoError(t, err)

	certPEM2, keyPEM2 := generateTestCA(t)
	c2, err := ca.NewFromPEM(certPEM2, keyPEM2, 24*time.Hour)
	require.NoError(t, err)

	// Issue cert with CA1
	csrPEM, _ := generateCSR(t, "device-001")
	issued, err := c1.IssueCertificateFromCSR("device-001", csrPEM)
	require.NoError(t, err)

	// Verify with CA2 should fail
	_, err = c2.VerifyCertificate(issued.CertPEM)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "verification failed")
}

func TestVerifyCertificate_InvalidPEM(t *testing.T) {
	certPEM, keyPEM := generateTestCA(t)
	c, err := ca.NewFromPEM(certPEM, keyPEM, 24*time.Hour)
	require.NoError(t, err)

	_, err = c.VerifyCertificate([]byte("not a certificate"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "decode certificate PEM")
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

func TestFingerprintFromPEM(t *testing.T) {
	certPEM, keyPEM := generateTestCA(t)
	c, err := ca.NewFromPEM(certPEM, keyPEM, 24*time.Hour)
	require.NoError(t, err)

	csrPEM, _ := generateCSR(t, "device-001")
	issued, err := c.IssueCertificateFromCSR("device-001", csrPEM)
	require.NoError(t, err)

	fp, err := ca.FingerprintFromPEM(issued.CertPEM)
	require.NoError(t, err)
	assert.Equal(t, issued.Fingerprint, fp)
}

func TestFingerprintFromPEM_InvalidPEM(t *testing.T) {
	_, err := ca.FingerprintFromPEM([]byte("not a certificate"))
	assert.Error(t, err)
}

func TestDeviceIDFromPEM(t *testing.T) {
	certPEM, keyPEM := generateTestCA(t)
	c, err := ca.NewFromPEM(certPEM, keyPEM, 24*time.Hour)
	require.NoError(t, err)

	csrPEM, _ := generateCSR(t, "device-002")
	issued, err := c.IssueCertificateFromCSR("device-002", csrPEM)
	require.NoError(t, err)

	deviceID, err := ca.DeviceIDFromPEM(issued.CertPEM)
	require.NoError(t, err)
	assert.Equal(t, "device-002", deviceID)
}

func TestDeviceIDFromPEM_InvalidPEM(t *testing.T) {
	_, err := ca.DeviceIDFromPEM([]byte("not a certificate"))
	assert.Error(t, err)
}

func TestTrustPool(t *testing.T) {
	certPEM, keyPEM := generateTestCA(t)
	c, err := ca.NewFromPEM(certPEM, keyPEM, 24*time.Hour)
	require.NoError(t, err)

	pool := c.TrustPool()
	assert.NotNil(t, pool)
}

// TestFingerprintFromCert_NilSafe pins the defensive nil guard — the gateway
// mTLS path must never panic computing a revocation fingerprint.
func TestFingerprintFromCert_NilSafe(t *testing.T) {
	assert.Empty(t, ca.FingerprintFromCert(nil))
}
