package mtls

import (
	"crypto/tls"
	"errors"
	"net/http"
)

func DeviceIDFromRequest(r *http.Request) (string, error) {
	if r.TLS == nil {
		return "", errors.New("no TLS connection")
	}

	if len(r.TLS.PeerCertificates) == 0 {
		return "", errors.New("no client certificate")
	}

	cert := r.TLS.PeerCertificates[0]
	deviceID := cert.Subject.CommonName

	if deviceID == "" {
		return "", errors.New("certificate CN is empty")
	}

	return deviceID, nil
}

func DeviceIDFromTLS(state *tls.ConnectionState) (string, error) {
	if state == nil {
		return "", errors.New("no TLS connection state")
	}

	if len(state.PeerCertificates) == 0 {
		return "", errors.New("no client certificate")
	}

	cert := state.PeerCertificates[0]
	deviceID := cert.Subject.CommonName

	if deviceID == "" {
		return "", errors.New("certificate CN is empty")
	}

	return deviceID, nil
}
