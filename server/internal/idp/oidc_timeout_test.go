package idp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBoundedOIDCClient_IsBounded(t *testing.T) {
	c := newBoundedOIDCClient()
	require.NotNil(t, c)
	assert.Positive(t, c.Timeout, "overall client timeout must be set")
	tr, ok := c.Transport.(*http.Transport)
	require.True(t, ok)
	assert.Positive(t, tr.TLSHandshakeTimeout)
	assert.Positive(t, tr.ResponseHeaderTimeout)
}

func TestNewOIDCProvider_DiscoveryRespectsTimeout(t *testing.T) {

	orig := oidcHTTPTimeout
	oidcHTTPTimeout = 400 * time.Millisecond
	t.Cleanup(func() { oidcHTTPTimeout = orig })

	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		<-block
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(block) })

	done := make(chan error, 1)
	go func() {
		_, err := NewOIDCProvider(context.Background(), ProviderConfig{
			IssuerURL: srv.URL,
			ClientID:  "test",
		})
		done <- err
	}()

	select {
	case err := <-done:
		assert.Error(t, err, "discovery against a hanging server must fail, not succeed")
	case <-time.After(5 * time.Second):
		t.Fatal("NewOIDCProvider hung past the bounded timeout — discovery is not bounded")
	}
}
