package contract

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

// X.509 fixtures for the loopback mTLS tests.
//
// The sdk module has an exported equivalent (sdk/cryptotest) that the agent
// and the sdk's own crypto tests share. The contract deliberately does NOT
// import it: contract is a leaf module, and a test-only edge to sdk would
// still put the whole system-management SDK into the build list of a module
// whose entire purpose is that a third party can implement the wire protocol
// without inheriting anything else. So the two constructors the contract's
// mTLS tests need live here, unexported, and the duplication is the price of
// leaf purity rather than an oversight.
//
// Fixed validity bounds, no time.Now(): the same clock-seam rule the sdk copy
// documents. A wide fixed window is "valid now" for any realistic test run and
// stays inside the ASN.1 GeneralizedTime range.
var (
	testCertNotBefore = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	testCertNotAfter  = time.Date(2200, 1, 1, 0, 0, 0, 0, time.UTC)
)

// testCertSerial hands out unique-per-process serial numbers so several certs
// in one run cannot collide.
var testCertSerial atomic.Int64

func nextTestCertSerial() *big.Int { return big.NewInt(testCertSerial.Add(1)) }

// genCA returns a self-signed test CA: its PEM-encoded certificate, its private
// key, and the parsed certificate (for signing leaves and building pools).
func genCA(t testing.TB, commonName string) (caPEM []byte, key *ecdsa.PrivateKey, cert *x509.Certificate) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ca key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber:          nextTestCertSerial(),
		Subject:               pkix.Name{CommonName: commonName},
		NotBefore:             testCertNotBefore,
		NotAfter:              testCertNotAfter,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create ca: %v", err)
	}
	cert, err = x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse ca: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), key, cert
}

// genLeaf issues a leaf certificate signed by ca/caKey and returns the
// PEM-encoded certificate and key. Server leaves carry localhost SANs
// (127.0.0.1, ::1, localhost) so they work for httptest TLS servers.
func genLeaf(t testing.TB, ca *x509.Certificate, caKey *ecdsa.PrivateKey, commonName string, server bool) (certPEM, keyPEM []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("leaf key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: nextTestCertSerial(),
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    testCertNotBefore,
		NotAfter:     testCertNotAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
	}
	if server {
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		template.IPAddresses = []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}
		template.DNSNames = []string{"localhost"}
	} else {
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create leaf: %v", err)
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal leaf key: %v", err)
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM
}
