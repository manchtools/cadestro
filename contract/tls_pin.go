package contract

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
)

// WithCAPin requires the TLS server chain to contain a certificate whose DER
// fingerprint matches pin. The check runs during the handshake, before an
// HTTP request (and therefore any bearer token) can be written.
func WithCAPin(pin string) ClientOption {
	return &funcOption{func(_ *Client, httpClient **http.Client) {
		base := *httpClient
		if base == nil {
			base = bootstrapHTTPClient()
		}
		transport, ok := base.Transport.(*http.Transport)
		if !ok {
			*httpClient = &http.Client{Transport: roundTripError{"CA pinning requires an HTTP transport"}, Timeout: base.Timeout}
			return
		}
		transport = transport.Clone()
		tlsConfig := transport.TLSClientConfig
		if tlsConfig == nil {
			tlsConfig = &tls.Config{MinVersion: tls.VersionTLS13}
		} else {
			tlsConfig = tlsConfig.Clone()
		}
		previous := tlsConfig.VerifyConnection
		tlsConfig.VerifyConnection = func(state tls.ConnectionState) error {
			if previous != nil {
				if err := previous(state); err != nil {
					return err
				}
			}
			for _, chain := range state.VerifiedChains {
				for _, cert := range chain {
					if certificateFingerprint(cert) == pin {
						return nil
					}
				}
			}
			for _, cert := range state.PeerCertificates {
				if certificateFingerprint(cert) == pin {
					return nil
				}
			}
			return fmt.Errorf("TLS server certificate does not match the supplied CA pin")
		}
		transport.TLSClientConfig = tlsConfig
		clone := *base
		clone.Transport = transport
		*httpClient = &clone
	}}
}

func certificateFingerprint(cert *x509.Certificate) string {
	sum := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(sum[:])
}

type roundTripError struct{ message string }

func (e roundTripError) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New(e.message)
}
