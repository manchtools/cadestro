//go:build integration

package timesync_test

import (
	"context"
	"os"
	"testing"

	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
	"github.com/manchtools/cadestro/sdk/sys/timesync"
)

func systemdRunning() bool {
	_, err := os.Stat("/run/systemd/system")
	return err == nil
}

func TestStatus_Integration(t *testing.T) {
	backends := timesync.Detect(context.Background())
	if len(backends) == 0 {
		t.Skip("no time-sync backend on PATH")
	}
	r, err := sysexec.NewRunner(sysexec.Direct)
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}
	for _, b := range backends {
		m, err := timesync.New(b, r)
		if err != nil {
			t.Fatalf("New(%v): %v", b, err)
		}
		status, err := m.Status(context.Background())
		switch {
		case b == timesync.Timedatectl && systemdRunning():

			if err != nil {
				t.Fatalf("Timedatectl Status under systemd: %v", err)
			}
			t.Logf("Timedatectl Status = %+v", status)
		case err != nil:

			t.Logf("Status(%v): %v", b, err)
		}
	}
}
