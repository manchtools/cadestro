package crypto_test

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/manchtools/cadestro/server/internal/crypto"
)

func TestEncryptorRoundTripAndContextBinding(t *testing.T) {
	key := hex.EncodeToString(make([]byte, 32))
	encryptor, err := crypto.NewEncryptor(key)
	if err != nil {
		t.Fatal(err)
	}
	aad := crypto.RowAAD("provider-a", crypto.PurposeIdPClientSecret)
	ciphertext, err := encryptor.EncryptWithContext("secret", aad)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(ciphertext, "enc:v1:") {
		t.Fatalf("ciphertext prefix = %q", ciphertext)
	}
	plaintext, err := encryptor.DecryptWithContext(ciphertext, aad)
	if err != nil {
		t.Fatal(err)
	}
	if plaintext != "secret" {
		t.Fatalf("plaintext = %q", plaintext)
	}
	if _, err := encryptor.DecryptWithContext(ciphertext, crypto.RowAAD("provider-b", crypto.PurposeIdPClientSecret)); err == nil {
		t.Fatal("wrong context decrypted ciphertext")
	}
}

func TestEncryptorRejectsUnsafeInputs(t *testing.T) {
	if _, err := crypto.NewEncryptor(""); err == nil {
		t.Fatal("empty key accepted")
	}
	encryptor, err := crypto.NewEncryptor(hex.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := encryptor.EncryptWithContext("secret", nil); err == nil {
		t.Fatal("empty AAD accepted")
	}
	if _, err := encryptor.DecryptWithContext("plaintext", []byte("aad")); err == nil {
		t.Fatal("plaintext value accepted")
	}
	if _, err := encryptor.DecryptWithContext("enc:v2:value", []byte("aad")); err == nil {
		t.Fatal("unknown encrypted format accepted")
	}
}
