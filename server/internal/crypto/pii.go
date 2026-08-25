package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
)

const piiPrefix = "pii:v1:"

const PurposeUserDEK = "user-dek"

type DEK struct {
	gcm cipher.AEAD
}

func GenerateWrappedDEK(kek *Encryptor, userID string) (string, error) {
	if kek == nil {
		return "", errors.New("crypto: refusing to mint a DEK without a KEK — the wrapped key would be stored unprotected")
	}
	if userID == "" {
		return "", errors.New("crypto: refusing to mint a DEK without an owning user id")
	}
	raw := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, raw); err != nil {
		return "", fmt.Errorf("generate DEK: %w", err)
	}
	wrapped, err := kek.EncryptWithContext(base64.StdEncoding.EncodeToString(raw), RowAAD(userID, PurposeUserDEK))
	if err != nil {
		return "", fmt.Errorf("wrap DEK: %w", err)
	}
	return wrapped, nil
}

func UnwrapDEK(kek *Encryptor, userID, wrapped string) (*DEK, error) {
	if kek == nil {
		return nil, errors.New("crypto: cannot unwrap a DEK without a KEK")
	}
	b64, err := kek.DecryptWithContext(wrapped, RowAAD(userID, PurposeUserDEK))
	if err != nil {
		return nil, fmt.Errorf("unwrap DEK for %s: %w", userID, err)
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil || len(raw) != 32 {
		return nil, fmt.Errorf("unwrap DEK for %s: invalid key material", userID)
	}
	block, err := aes.NewCipher(raw)
	if err != nil {
		return nil, fmt.Errorf("unwrap DEK for %s: %w", userID, err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("unwrap DEK for %s: %w", userID, err)
	}
	return &DEK{gcm: gcm}, nil
}

func (d *DEK) SealField(plaintext, field string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	if field == "" {
		return "", errors.New("crypto: refusing to seal PII without a field binding")
	}
	nonce := make([]byte, d.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	ct := d.gcm.Seal(nonce, nonce, []byte(plaintext), []byte(field))
	return piiPrefix + base64.StdEncoding.EncodeToString(ct), nil
}

func (d *DEK) OpenField(value, field string) (string, error) {
	if value == "" {
		return "", nil
	}
	if !strings.HasPrefix(value, piiPrefix) {
		return "", errors.New("crypto: plaintext PII rejected")
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, piiPrefix))
	if err != nil {
		return "", fmt.Errorf("decode PII ciphertext: %w", err)
	}
	nonceSize := d.gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("PII ciphertext too short")
	}
	nonce, ct := data[:nonceSize], data[nonceSize:]
	pt, err := d.gcm.Open(nil, nonce, ct, []byte(field))
	if err != nil {
		return "", fmt.Errorf("open PII field %s: %w", field, err)
	}
	return string(pt), nil
}
