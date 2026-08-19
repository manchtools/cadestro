package agentstream

import (
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manchtools/cadestro/server/internal/mtls"
)

func TestMTLSMiddlewareFailsClosedAndBindsAgentIdentity(t *testing.T) {
	deviceID := ulid.Make().String()
	agentURI, err := mtls.PeerClassURI(mtls.PeerClassAgent)
	require.NoError(t, err)
	controlURI, err := mtls.PeerClassURI(mtls.PeerClassControl)
	require.NoError(t, err)
	cert := func(uri *url.URL) *x509.Certificate {
		return &x509.Certificate{
			Raw: []byte("certificate"), Subject: pkix.Name{CommonName: deviceID}, URIs: []*url.URL{uri},
		}
	}
	next := http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		got, ok := DeviceIDFromContext(request.Context())
		if !ok || got != deviceID {
			http.Error(w, "identity missing", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	call := func(peer *x509.Certificate, path string) int {
		request := httptest.NewRequest(http.MethodPost, path, nil)
		if peer != nil {
			request.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{peer}}
		}
		response := httptest.NewRecorder()
		MTLSMiddleware(next).ServeHTTP(response, request)
		return response.Code
	}

	assert.Equal(t, http.StatusNoContent, call(cert(agentURI), "/stream"))
	assert.Equal(t, http.StatusUnauthorized, call(nil, "/stream"))
	assert.Equal(t, http.StatusForbidden, call(cert(controlURI), "/stream"))

	health := httptest.NewRequest(http.MethodGet, "/health", nil)
	healthResponse := httptest.NewRecorder()
	MTLSMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })).
		ServeHTTP(healthResponse, health)
	assert.Equal(t, http.StatusOK, healthResponse.Code)
}
