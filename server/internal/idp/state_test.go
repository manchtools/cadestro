package idp

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateState_Nonce_CodeVerifier_EntropyAndShape(t *testing.T) {
	gens := map[string]func() (string, error){
		"state":         GenerateState,
		"nonce":         GenerateNonce,
		"code_verifier": GenerateCodeVerifier,
	}
	for name, gen := range gens {
		t.Run(name, func(t *testing.T) {
			a, err := gen()
			require.NoError(t, err)
			b, err := gen()
			require.NoError(t, err)

			assert.NotEmpty(t, a)
			assert.NotEqual(t, a, b, "two successive %s values must differ (entropy)", name)

			raw, err := base64.RawURLEncoding.DecodeString(a)
			require.NoErrorf(t, err, "%s must be RawURLEncoding", name)
			assert.Lenf(t, raw, 32, "%s must carry 32 bytes of entropy", name)
		})
	}
}

func TestCodeChallengeS256_KnownAnswer(t *testing.T) {
	const (
		verifier  = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
		challenge = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	)
	assert.Equal(t, challenge, CodeChallengeS256(verifier))
}
