package crypto

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	sdkcrypto "github.com/manchtools/cadestro/sdk/crypto"
)

const prefix = "enc:v1:"

const (
	PurposeIdPClientSecret              = "idp-client-secret"
	PurposeActionEncryptionPresharedKey = "action-encryption-preshared-key"
	PurposeActionWifiPSK                = "action-wifi-psk"
	PurposeActionWifiClientKey          = "action-wifi-client-key"
)

func IsEncryptedValue(value string) bool { return strings.HasPrefix(value, prefix) }

func DeviceSecretAAD(rowID, deviceID, kind, subject string, version uint32) []byte {
	return []byte(fmt.Sprintf("%s|%s|%s|%s|v%d", rowID, deviceID, kind, subject, version))
}

func RowAAD(rowID, purpose string) []byte {
	return []byte(rowID + "|" + purpose)
}

type Encryptor struct {
	key []byte
}

func NewEncryptor(keyHex string) (*Encryptor, error) {
	if keyHex == "" {
		return nil, errors.New("encryption key is required")
	}

	key, err := hex.DecodeString(keyHex)
	if err != nil {
		return nil, fmt.Errorf("invalid encryption key: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("encryption key must be 32 bytes (64 hex chars), got %d bytes", len(key))
	}

	return &Encryptor{key: append([]byte(nil), key...)}, nil
}

func (e *Encryptor) EncryptWithContext(plaintext string, aad []byte) (string, error) {
	if e == nil {
		return "", errors.New("crypto: encryptor is required")
	}
	if len(aad) == 0 {
		return "", errors.New("crypto: AAD context is required")
	}
	if plaintext == "" {
		return "", nil
	}

	ciphertext, err := sdkcrypto.SealWithAAD(e.key, []byte(plaintext), aad)
	if err != nil {
		return "", fmt.Errorf("encrypt: %w", err)
	}
	return prefix + base64.StdEncoding.EncodeToString(ciphertext), nil
}

func (e *Encryptor) DecryptWithContext(value string, aad []byte) (string, error) {
	if e == nil {
		return "", errors.New("crypto: encryptor is required")
	}
	if len(aad) == 0 {
		return "", errors.New("crypto: AAD context is required")
	}
	if value == "" {
		return "", nil
	}
	switch {
	case strings.HasPrefix(value, prefix):
		data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, prefix))
		if err != nil {
			return "", fmt.Errorf("decode ciphertext: %w", err)
		}
		plaintext, err := sdkcrypto.OpenWithAAD(e.key, data, aad)
		if err != nil {
			return "", fmt.Errorf("decrypt: %w", err)
		}
		return string(plaintext), nil
	case strings.HasPrefix(value, "enc:"):

		tag := value
		if i := strings.Index(value[len("enc:"):], ":"); i >= 0 {
			tag = value[:len("enc:")+i]
		}
		return "", fmt.Errorf("crypto: unsupported at-rest format %q", tag)
	default:
		return "", errors.New("crypto: plaintext value rejected")
	}
}
