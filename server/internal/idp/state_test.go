package idp

import (
	"crypto/rand"
	"encoding/base64"
	"io"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateTransactionValues_EntropyAndShape(t *testing.T) {
	state, nonce, verifier, err := GenerateTransactionValues()
	require.NoError(t, err)
	state2, nonce2, verifier2, err := GenerateTransactionValues()
	require.NoError(t, err)
	values := []string{state, nonce, verifier}
	for i, value := range values {
		assert.NotEmpty(t, value)
		raw, err := base64.RawURLEncoding.DecodeString(value)
		require.NoErrorf(t, err, "value %d must be RawURLEncoding", i)
		assert.Lenf(t, raw, 32, "value %d must carry 32 bytes of entropy", i)
	}
	assert.NotEqual(t, state, nonce)
	assert.NotEqual(t, state, verifier)
	assert.NotEqual(t, nonce, verifier)
	assert.NotEqual(t, state, state2)
	assert.NotEqual(t, nonce, nonce2)
	assert.NotEqual(t, verifier, verifier2)
}

func TestGenerateTransactionValues_EntropyFailure(t *testing.T) {
	original := rand.Reader
	rand.Reader = iotest.ErrReader(io.ErrUnexpectedEOF)
	t.Cleanup(func() { rand.Reader = original })

	_, _, _, err := GenerateTransactionValues()
	assert.ErrorContains(t, err, "generate OIDC transaction values")
}
