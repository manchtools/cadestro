//go:build integration

package desktop_test

import (
	"context"
	"os"
	osexec "os/exec"
	"testing"

	"github.com/manchtools/cadestro/sdk/sys/desktop"
	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
)

func systemdRunning() bool {
	_, err := os.Stat("/run/systemd/system")
	return err == nil
}

func TestActiveSessions_Integration(t *testing.T) {
	if _, err := osexec.LookPath("loginctl"); err != nil {
		t.Skip("loginctl not present; logind path not exercisable")
	}
	r, err := sysexec.NewRunner(sysexec.Direct)
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}
	m, err := desktop.New(r)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	sessions, err := m.ActiveSessions(context.Background())

	for _, s := range sessions {
		if s.Type != "x11" && s.Type != "wayland" && s.Type != "mir" {
			t.Errorf("ActiveSessions returned a non-graphical session type %q", s.Type)
		}
	}

	if systemdRunning() {

		if err != nil {
			t.Fatalf("ActiveSessions against real logind: %v", err)
		}
		t.Logf("ActiveSessions returned %d graphical session(s)", len(sessions))
		return
	}

	if err != nil {
		t.Skipf("loginctl present but logind not reachable here: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("without a running logind, ActiveSessions should be empty, got %d", len(sessions))
	}
}
