package main

import (
	"io"
	"log/slog"
	"testing"

	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestRootBackend(t *testing.T) {
	if _, err := rootBackend(1000); err == nil {
		t.Fatal("non-root agent accepted")
	}
	backend, err := rootBackend(0)
	if err != nil {
		t.Fatal(err)
	}
	if backend != sysexec.Direct {
		t.Fatalf("backend = %v", backend)
	}
}
