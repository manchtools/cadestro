package credentials

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSavePermissions(t *testing.T) {
	requireMachineID(t)
	dir := t.TempDir()
	store := NewStore(dir)
	if err := store.Save(context.Background(), sampleCreds()); err != nil {
		t.Fatalf("Save: %v", err)
	}

	for _, f := range []string{credentialsFile, saltFile} {
		info, err := os.Stat(filepath.Join(dir, f))
		if err != nil {
			t.Fatalf("stat %s: %v", f, err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Errorf("%s mode = %v, want 0600", f, info.Mode().Perm())
		}
	}
	di, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if di.Mode().Perm() != 0o700 {
		t.Errorf("store dir mode = %v, want 0700", di.Mode().Perm())
	}
}

func TestLoadSubstitutedSaltFails(t *testing.T) {
	requireMachineID(t)
	dir := t.TempDir()
	store := NewStore(dir)
	if err := store.Save(context.Background(), sampleCreds()); err != nil {
		t.Fatalf("Save: %v", err)
	}

	subst := make([]byte, saltLen)
	for i := range subst {
		subst[i] = 0xAB
	}
	if err := os.WriteFile(filepath.Join(dir, saltFile), subst, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(); err == nil {
		t.Fatal("expected Load to fail with a substituted salt (different derived key)")
	}
}

func TestLoadCrossMachineFails(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	prev := getMachineID
	t.Cleanup(func() { getMachineID = prev })

	getMachineID = func() ([]byte, error) { return []byte("machine-id-AAAAAAAAAAAA"), nil }
	if err := store.Save(context.Background(), sampleCreds()); err != nil {
		t.Fatalf("Save (machine A): %v", err)
	}

	getMachineID = func() ([]byte, error) { return []byte("machine-id-BBBBBBBBBBBB"), nil }
	if _, err := store.Load(); err == nil {
		t.Fatal("expected Load to fail under a different machine ID")
	}
}

func TestLoadTruncatedCiphertextTooShort(t *testing.T) {
	requireMachineID(t)
	dir := t.TempDir()
	store := NewStore(dir)
	if err := store.Save(context.Background(), sampleCreds()); err != nil {
		t.Fatalf("Save: %v", err)
	}

	short := append([]byte(credentialsMagicV1), []byte("xx")...)
	if err := os.WriteFile(filepath.Join(dir, credentialsFile), short, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := store.Load()
	if err == nil || !strings.Contains(err.Error(), "too short") {
		t.Fatalf("expected 'ciphertext too short', got: %v", err)
	}
}

func TestRefusesWritableStoreDir(t *testing.T) {
	requireMachineID(t)
	dir := t.TempDir()
	store := NewStore(dir)
	if err := store.Save(context.Background(), sampleCreds()); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := os.Chmod(dir, 0o777); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	_, err := store.Load()
	if err == nil || !strings.Contains(err.Error(), "writable") {
		t.Fatalf("expected refusal of a world-writable store dir, got: %v", err)
	}
}

func TestSaveTightensLooseDir(t *testing.T) {
	requireMachineID(t)
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o775); err != nil {
		t.Fatal(err)
	}
	store := NewStore(dir)
	if err := store.Save(context.Background(), sampleCreds()); err != nil {
		t.Fatalf("Save should tighten and succeed, got: %v", err)
	}
	di, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if di.Mode().Perm() != 0o700 {
		t.Errorf("Save did not tighten the dir to 0700, got %v", di.Mode().Perm())
	}
}
