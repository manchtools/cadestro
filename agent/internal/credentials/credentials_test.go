package credentials

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func sampleCreds() *Credentials {
	return &Credentials{
		DeviceID:           "01HXYZSAMPLE",
		CACert:             []byte("ca-cert-bytes"),
		Certificate:        []byte("client-cert-bytes"),
		PendingCertificate: []byte("pending-cert-bytes"),
		PendingPrivateKey:  []byte("pending-key-bytes"),
		PendingCSR:         []byte("pending-csr-bytes"),
		PrivateKey:         []byte("client-key-bytes"),
		AgentAddr:          "https://agent.control.example.test:443",
	}
}

func TestPlaintextCredentialsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	in := sampleCreds()

	if err := store.Save(context.Background(), in); err != nil {
		t.Fatalf("Save: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, credentialsFile))
	if err != nil {
		t.Fatalf("read credentials file: %v", err)
	}
	var decoded Credentials
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("credentials file is not plaintext JSON: %v", err)
	}
	if !reflect.DeepEqual(decoded, *in) {
		t.Fatalf("decoded credentials do not match input")
	}
	out, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(out, in) {
		t.Fatalf("loaded credentials do not match input")
	}
}

func TestCredentialsReady(t *testing.T) {
	var nilCredentials *Credentials
	if nilCredentials.Ready() {
		t.Fatal("nil credentials reported ready")
	}
	base := &Credentials{DeviceID: "device", CACert: []byte("ca"), Certificate: []byte("cert"), PrivateKey: []byte("key"), AgentAddr: "https://agent.example.test"}
	for name, mutate := range map[string]func(*Credentials){
		"ready":                       func(*Credentials) {},
		"pending certificate allowed": func(c *Credentials) { c.PendingCertificate = []byte("pending-cert") },
		"missing device":              func(c *Credentials) { c.DeviceID = "" },
		"missing ca":                  func(c *Credentials) { c.CACert = nil },
		"missing certificate":         func(c *Credentials) { c.Certificate = nil },
		"missing private key":         func(c *Credentials) { c.PrivateKey = nil },
		"missing agent address":       func(c *Credentials) { c.AgentAddr = "" },
		"pending csr":                 func(c *Credentials) { c.PendingCSR = []byte("csr") },
		"pending key":                 func(c *Credentials) { c.PendingPrivateKey = []byte("key") },
	} {
		t.Run(name, func(t *testing.T) {
			got := *base

			mutate(&got)
			want := name == "ready" || name == "pending certificate allowed"
			if got.Ready() != want {
				t.Fatalf("Ready() = %v, want %v", got.Ready(), want)
			}
		})
	}
}

func TestSaveIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	in := sampleCreds()
	if err := store.Save(context.Background(), in); err != nil {
		t.Fatalf("first Save: %v", err)
	}
	if err := store.Save(context.Background(), in); err != nil {
		t.Fatalf("second Save: %v", err)
	}
	if _, err := store.Load(); err != nil {
		t.Fatalf("Load after double Save: %v", err)
	}
}

func TestCredentialsPermissions(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	if err := store.Save(context.Background(), sampleCreds()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	fileInfo, err := os.Stat(filepath.Join(dir, credentialsFile))
	if err != nil {
		t.Fatalf("stat credentials file: %v", err)
	}
	if fileInfo.Mode().Perm() != 0o600 {
		t.Fatalf("credentials mode = %o, want 600", fileInfo.Mode().Perm())
	}
	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat credentials directory: %v", err)
	}
	if dirInfo.Mode().Perm() != 0o700 {
		t.Fatalf("credentials directory mode = %o, want 700", dirInfo.Mode().Perm())
	}
}

func TestLoadRejectsUnsafePermissions(t *testing.T) {
	t.Run("file", func(t *testing.T) {
		dir := t.TempDir()
		store := NewStore(dir)
		if err := store.Save(context.Background(), sampleCreds()); err != nil {
			t.Fatalf("Save: %v", err)
		}
		if err := os.Chmod(filepath.Join(dir, credentialsFile), 0o644); err != nil {
			t.Fatalf("chmod credentials file: %v", err)
		}
		if _, err := store.Load(); err == nil {
			t.Fatal("Load accepted a group-readable credentials file")
		}
	})
	t.Run("directory", func(t *testing.T) {
		dir := t.TempDir()
		store := NewStore(dir)
		if err := store.Save(context.Background(), sampleCreds()); err != nil {
			t.Fatalf("Save: %v", err)
		}
		if err := os.Chmod(dir, 0o755); err != nil {
			t.Fatalf("chmod credentials directory: %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
		if _, err := store.Load(); err == nil {
			t.Fatal("Load accepted a world-readable credentials directory")
		}
	})
}

func TestLoadRejectsCorruptJSON(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	if err := os.WriteFile(filepath.Join(dir, credentialsFile), []byte("not json"), 0o600); err != nil {
		t.Fatalf("write credentials file: %v", err)
	}
	if _, err := store.Load(); err == nil {
		t.Fatal("Load accepted corrupt JSON")
	}
}
