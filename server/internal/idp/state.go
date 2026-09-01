package idp

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
)

func GenerateTransactionValues() (state, nonce, codeVerifier string, err error) {
	b := make([]byte, 96)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", "", "", fmt.Errorf("generate OIDC transaction values: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b[:32]),
		base64.RawURLEncoding.EncodeToString(b[32:64]),
		base64.RawURLEncoding.EncodeToString(b[64:]), nil
}
