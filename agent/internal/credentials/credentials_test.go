package credentials

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func requireMachineID(t *testing.T) {
	t.Helper()
	if _, err := getMachineID(); err != nil {
		t.Skipf("machine ID not available: %v", err)
	}
}

func sampleCreds() *Credentials {
	return &Credentials{
		DeviceID:    "01HXYZSAMPLE",
		CACert:      []byte("ca-cert-bytes"),
		Certificate: []byte("client-cert-bytes"),
		PrivateKey:  []byte("client-key-bytes"),
		AgentAddr:   "https://agent.control.example.test:443",
		ControlAddr: "https://control.example.test:443",
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	requireMachineID(t)

	dir := t.TempDir()
	store := NewStore(dir)

	in := sampleCreds()
	if err := store.Save(in); err != nil {
		t.Fatalf("Save: %v", err)
	}

	out, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if out.DeviceID != in.DeviceID {
		t.Errorf("DeviceID mismatch: got %q want %q", out.DeviceID, in.DeviceID)
	}
	if !bytes.Equal(out.CACert, in.CACert) {
		t.Errorf("CACert mismatch")
	}
	if !bytes.Equal(out.Certificate, in.Certificate) {
		t.Errorf("Certificate mismatch")
	}
	if !bytes.Equal(out.PrivateKey, in.PrivateKey) {
		t.Errorf("PrivateKey mismatch")
	}
	if out.AgentAddr != in.AgentAddr {
		t.Errorf("AgentAddr mismatch: got %q want %q", out.AgentAddr, in.AgentAddr)
	}
	if out.ControlAddr != in.ControlAddr {
		t.Errorf("ControlAddr mismatch: got %q want %q", out.ControlAddr, in.ControlAddr)
	}
}

func TestSaveIsIdempotent(t *testing.T) {
	requireMachineID(t)

	dir := t.TempDir()
	store := NewStore(dir)

	in := sampleCreds()
	if err := store.Save(in); err != nil {
		t.Fatalf("first Save: %v", err)
	}
	if err := store.Save(in); err != nil {
		t.Fatalf("second Save: %v", err)
	}

	out, err := store.Load()
	if err != nil {
		t.Fatalf("Load after double Save: %v", err)
	}
	if out.DeviceID != in.DeviceID {
		t.Errorf("DeviceID mismatch after double Save")
	}
}

func TestSaveWritesV1Magic(t *testing.T) {
	requireMachineID(t)

	dir := t.TempDir()
	store := NewStore(dir)

	if err := store.Save(sampleCreds()); err != nil {
		t.Fatalf("Save: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, credentialsFile))
	if err != nil {
		t.Fatalf("read credentials file: %v", err)
	}
	if !bytes.HasPrefix(raw, []byte(credentialsMagicV1)) {
		t.Errorf("credentials file missing v1 magic prefix; first 16 bytes = %q", raw[:min(16, len(raw))])
	}
}

func TestLoadRejectsUnknownFutureMagic(t *testing.T) {
	requireMachineID(t)

	dir := t.TempDir()
	store := NewStore(dir)

	if _, err := store.loadOrCreateSalt(); err != nil {
		t.Fatalf("loadOrCreateSalt: %v", err)
	}

	credPath := filepath.Join(dir, credentialsFile)
	if err := os.WriteFile(credPath, []byte("cadestrocred:v999:opaque"), 0600); err != nil {
		t.Fatalf("write fake creds: %v", err)
	}

	_, err := store.Load()
	if err == nil {
		t.Fatal("expected Load to reject unknown future magic, got nil error")
	}

	if msg := err.Error(); !strings.Contains(msg, "re-enroll") {
		t.Errorf("error message missing re-enroll hint: %q", msg)
	}
}

func TestLoadCorruptCiphertextFails(t *testing.T) {
	requireMachineID(t)

	dir := t.TempDir()
	store := NewStore(dir)

	if err := store.Save(sampleCreds()); err != nil {
		t.Fatalf("Save: %v", err)
	}

	credPath := filepath.Join(dir, credentialsFile)
	raw, err := os.ReadFile(credPath)
	if err != nil {
		t.Fatalf("read credentials file: %v", err)
	}

	if len(raw) < len(credentialsMagicV1)+8 {
		t.Fatalf("ciphertext too short to corrupt: %d bytes", len(raw))
	}
	raw[len(raw)-1] ^= 0xFF
	if err := os.WriteFile(credPath, raw, 0600); err != nil {
		t.Fatalf("rewrite credentials file: %v", err)
	}

	if _, err := store.Load(); err == nil {
		t.Error("expected Load to fail on corrupted ciphertext, got nil error")
	}
}

func TestLoadMissingSaltFails(t *testing.T) {
	requireMachineID(t)

	dir := t.TempDir()
	store := NewStore(dir)

	if _, err := store.Load(); err == nil {
		t.Error("expected Load with missing salt to fail, got nil error")
	}
}

func TestExistsAndDelete(t *testing.T) {
	requireMachineID(t)

	dir := t.TempDir()
	store := NewStore(dir)

	if store.Exists() {
		t.Fatal("Exists returned true for empty data dir")
	}

	if err := store.Save(sampleCreds()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !store.Exists() {
		t.Fatal("Exists returned false after Save")
	}

	if err := store.Delete(); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if store.Exists() {
		t.Fatal("Exists returned true after Delete")
	}
}
