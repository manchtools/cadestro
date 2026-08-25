package main

import (
	"context"
	"os"
	"testing"

	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
	"github.com/manchtools/cadestro/sdk/sys/exec/exectest"
)

func TestReconcileUnitAtStartup_NonRootIsCompleteNoop(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("test requires a non-root euid to exercise the root guard")
	}
	fake := exectest.New(sysexec.Direct)
	reconcileUnitAtStartup(context.Background(), fake, discardLogger(), "/var/lib/cadestro")
	if calls := fake.Calls(); len(calls) != 0 {
		t.Fatalf("non-root reconcile must be a complete no-op, ran %d commands: %+v", len(calls), calls)
	}
}

func TestRunInstallUnit_NonRootRefused(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("test requires a non-root euid to exercise the root guard")
	}
	if code := runInstallUnit([]string{}); code != 1 {
		t.Fatalf("runInstallUnit as non-root = exit %d, want 1", code)
	}
}
