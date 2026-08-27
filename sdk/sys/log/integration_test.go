//go:build integration

package log_test

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
	syslog "github.com/manchtools/cadestro/sdk/sys/log"
)

func systemdRunning() bool {
	_, err := os.Stat("/run/systemd/system")
	return err == nil
}

func newJournald(t *testing.T) syslog.Source {
	t.Helper()
	r, err := sysexec.NewRunner(sysexec.Direct)
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}
	s, err := syslog.New(syslog.Journald, r)
	if err != nil {
		t.Fatalf("New(Journald): %v", err)
	}
	return s
}

func TestQuery_Integration(t *testing.T) {
	for _, b := range syslog.Detect(context.Background()) {
		if b != syslog.Journald && b != syslog.Syslog {
			t.Errorf("Detect returned unexpected backend %v", b)
		}
	}
	if _, err := exec.LookPath("journalctl"); err != nil {
		t.Skip("journalctl not present")
	}
	s := newJournald(t)
	lines, err := s.Query(context.Background(), syslog.Query{Lines: 5})
	if systemdRunning() {
		if err != nil {
			t.Fatalf("journalctl Query under systemd: %v", err)
		}
		if len(lines) == 0 {
			t.Fatal("journalctl returned no lines under systemd (journal-group access missing, or empty journal)")
		}
		t.Logf("journalctl returned %d line(s)", len(lines))
		return
	}
	if err != nil {
		t.Skipf("journalctl read unusable here (no privilege/journal): %v", err)
	}
}

func TestQuery_GrepSeed_Integration(t *testing.T) {
	if !systemdRunning() {
		t.Skip("no live systemd journal to seed (needs the test-sys container)")
	}
	if _, err := exec.LookPath("logger"); err != nil {
		t.Skip("logger not present; cannot seed a journal entry")
	}

	marker := "CADESTRO-LOG-SEED-" + strconv.Itoa(os.Getpid())
	if out, err := exec.Command("logger", "-t", "cadestro-sdk-test", marker).CombinedOutput(); err != nil {
		t.Skipf("cannot seed journal via logger: %v\n%s", err, out)
	}

	s := newJournald(t)
	ctx := context.Background()

	var lines []string
	var lastErr error
	for i := 0; i < 20; i++ {
		var err error
		lines, err = s.Query(ctx, syslog.Query{Grep: marker, Lines: 50})
		if err != nil {
			lastErr, lines = err, nil
			time.Sleep(100 * time.Millisecond)
			continue
		}
		if len(lines) > 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if len(lines) == 0 {
		if lastErr != nil {
			t.Fatalf("Query{Grep} against real journalctl: %v", lastErr)
		}
		t.Fatalf("seeded marker %q never returned by Query{Grep} — real journalctl --grep filter drifted?", marker)
	}

	for _, ln := range lines {
		if !strings.Contains(ln, marker) {
			t.Errorf("Query{Grep:%q} returned a non-matching line %q — --grep is not filtering", marker, ln)
		}
	}

	absent := "CADESTRO-LOG-ABSENT-" + strconv.Itoa(os.Getpid())
	ghost, err := s.Query(ctx, syslog.Query{Grep: absent, Lines: 50})
	if err != nil {
		t.Fatalf("Query{Grep:absent}: %v", err)
	}
	if len(ghost) != 0 {
		t.Errorf("Query{Grep:%q} for an unlogged marker returned %d line(s) — --grep not excluding or status marker leaked: %v", absent, len(ghost), ghost)
	}
}
