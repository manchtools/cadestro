package credentials

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMachineID_RawBytesAreTheKDFContract(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	orig := getMachineID
	t.Cleanup(func() { getMachineID = orig })

	getMachineID = func() ([]byte, error) { return []byte("abc123\n"), nil }
	if err := s.Save(sampleCreds()); err != nil {
		t.Fatalf("Save: %v", err)
	}

	getMachineID = func() ([]byte, error) { return []byte("abc123"), nil }
	if _, err := s.Load(); err == nil {
		t.Fatal("credentials decrypted after the machine-id bytes changed by only a trailing newline — the raw-bytes KDF contract is broken")
	}

	getMachineID = func() ([]byte, error) { return []byte("abc123\n"), nil }
	if _, err := s.Load(); err != nil {
		t.Fatalf("original raw machine-id bytes must keep decrypting: %v", err)
	}
}

func TestLoadOrCreateSalt_CorruptSaltFailsClosed(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	if err := os.WriteFile(filepath.Join(dir, saltFile), []byte("short"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := s.loadOrCreateSalt()
	if err == nil {
		t.Fatal("corrupt salt must fail closed, not regenerate")
	}
	if !strings.Contains(err.Error(), "corrupt") {
		t.Fatalf("error must name the corruption, got: %v", err)
	}

	got, rerr := os.ReadFile(filepath.Join(dir, saltFile))
	if rerr != nil || string(got) != "short" {
		t.Fatalf("corrupt salt file must be preserved, got %q err=%v", got, rerr)
	}
}
