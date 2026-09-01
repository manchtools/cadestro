package main

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/manchtools/cadestro/agent/internal/credentials"
)

func TestEnrollRejectsNonRoot(t *testing.T) {
	if got := runEnroll(nil, 1000); got != 1 {
		t.Fatalf("runEnroll exit code = %d, want 1", got)
	}
}

func TestLoadCredentialsRequiresEnrollment(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	_, err := loadCredentials(credentials.NewStore(dir))
	if err == nil || !strings.Contains(err.Error(), "agent is not enrolled; run cadestrod enroll") {
		t.Fatalf("loadCredentials error = %v", err)
	}
}

func TestLoadCredentialsRejectsIncompleteEnrollment(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	store := credentials.NewStore(dir)
	if err := store.Save(context.Background(), &credentials.Credentials{PendingCSR: []byte("csr"), PendingPrivateKey: []byte("key")}); err != nil {
		t.Fatal(err)
	}
	if _, err := loadCredentials(store); err == nil || !strings.Contains(err.Error(), "agent is not enrolled; run cadestrod enroll") {
		t.Fatalf("loadCredentials error = %v", err)
	}
}
