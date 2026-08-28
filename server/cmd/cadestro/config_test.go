package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSessionKeyRequiresPrivatePermissions(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "session.pem")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encoded}), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadSessionKey(path); err == nil {
		t.Fatal("loadSessionKey() accepted group/world-readable key")
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadSessionKey(path)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.Equal(privateKey) {
		t.Fatal("loaded session key differs")
	}
}
