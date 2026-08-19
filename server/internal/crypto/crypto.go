// Package crypto provides application-level encryption for sensitive data
// stored in the database: LUKS passphrases, LPS passwords, IdP client secrets,
// and per-subject audit-detail keys.
//
// The at-rest format is AAD-bound AES-256-GCM under the prefix "enc:v1:". Every ciphertext is
// bound to its row context via additional authenticated data, so a
// DB-level attacker cannot relocate a secret from one row (or purpose)
// to another and have it decrypt. There is deliberately NO nil-AAD API:
// the naked Encrypt/Decrypt pair was removed so a new call site cannot
// regress to unbound ciphertext (a guard test additionally pins that
// AEAD primitives are not used outside this package).
//
// The encryption key is loaded by the control server from its configured secret
// file or deployment override.
package crypto

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	sdkcrypto "github.com/manchtools/cadestro/sdk/crypto"
)

// prefix identifies the single AAD-bound AES-256-GCM at-rest format.
// Values carrying any other format, including plaintext, fail closed.
const prefix = "enc:v1:"

// Purpose tags for RowAAD — shared constants so the write and read
// paths can never drift apart on the AAD purpose dimension.
const (
	PurposeIdPClientSecret              = "idp-client-secret"
	PurposeActionEncryptionPresharedKey = "action-encryption-preshared-key"
	PurposeActionWifiPSK                = "action-wifi-psk"
	PurposeActionWifiClientKey          = "action-wifi-client-key"
)

// IsEncryptedValue reports whether value uses the only supported at-rest
// envelope. It is a shape check for metadata-only read paths; authenticated
// validity is established only by DecryptWithContext at the execution sink.
func IsEncryptedValue(value string) bool { return strings.HasPrefix(value, prefix) }

// SecretAAD builds the additional-authenticated-data that binds a
// device-scoped at-rest secret to its row context. deviceID and actionID
// are ULIDs (Crockford base32 — they can never contain the '|'
// separator), and secretType is a fixed literal ("luks" / "lps"), so the
// concatenation is unambiguous.
func SecretAAD(deviceID, actionID, secretType string) []byte {
	return []byte(deviceID + "|" + actionID + "|" + secretType)
}

// SecretAADForRow is the one at-rest context for device-owned credentials.
// The order is deliberately row/device/kind/subject/version: the immutable
// row id prevents sibling rotation swaps, the device prevents cross-owner
// moves, kind prevents LUKS/LPS confusion, and the subject plus format version
// leave an explicit upgrade boundary. Existing callers pass actionID as the
// subject, preserving the old helper API while tightening its binding.
func SecretAADForRow(deviceID, actionID, secretType, discriminator string) []byte {
	return []byte(discriminator + "|" + deviceID + "|" + secretType + "|" + actionID + "|v1")
}

// LegacySecretAADForRow reads schema-v1 rows written before the generic row
// context. It is migration-only; new writes must use SecretAADForRow.
func LegacySecretAADForRow(deviceID, actionID, secretType, discriminator string) []byte {
	return []byte(deviceID + "|" + actionID + "|" + secretType + "|" + discriminator)
}

// DeviceSecretAAD makes the generic encrypted-row context explicit for new
// storage users. Keep the tiny string form so migration code can use the same
// AEAD without another persistence abstraction.
func DeviceSecretAAD(rowID, deviceID, kind, subject string, version uint32) []byte {
	return []byte(fmt.Sprintf("%s|%s|%s|%s|v%d", rowID, deviceID, kind, subject, version))
}

// RowAAD builds the AAD for a secret owned by a single row: the owning
// row's ULID plus a fixed purpose literal (see the Purpose* constants).
// Mirrors SecretAAD's unambiguous '|' concatenation; the two shapes
// cannot collide because SecretAAD always has three segments.
func RowAAD(rowID, purpose string) []byte {
	return []byte(rowID + "|" + purpose)
}

// Encryptor handles AES-256-GCM encryption and decryption of secret values.
type Encryptor struct {
	key []byte
}

// NewEncryptor creates a new Encryptor from a hex-encoded 32-byte key.
// An empty key is rejected; control cannot run without at-rest encryption.
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

// EncryptWithContext encrypts plaintext bound to aad and returns an
// "enc:v1:<base64>" string. The aad is authenticated (not stored in the
// ciphertext) — DecryptWithContext must be given the SAME aad to open
// it, so a secret sealed for one row context cannot be opened in
// another. A missing encryptor or empty AAD is refused. Empty plaintext remains
// empty, but non-empty plaintext always becomes ciphertext.
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

// DecryptWithContext decrypts an "enc:v1:<base64>" AAD-bound value.
//
//   - "enc:v1:" values open with the SAME aad they were sealed under;
//     a mismatched aad or tampered ciphertext fails GCM authentication.
//   - any other "enc:*" prefix is rejected;
//   - non-empty plaintext is rejected; and
//   - an empty value round-trips as empty.
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
		// Report only the format tag, never ciphertext bytes.
		tag := value
		if i := strings.Index(value[len("enc:"):], ":"); i >= 0 {
			tag = value[:len("enc:")+i]
		}
		return "", fmt.Errorf("crypto: unsupported at-rest format %q", tag)
	default:
		return "", errors.New("crypto: plaintext value rejected")
	}
}
