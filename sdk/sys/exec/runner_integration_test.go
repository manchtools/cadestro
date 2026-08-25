//go:build integration

package exec_test

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
)

func sudoRunner(t *testing.T) sysexec.Runner {
	t.Helper()
	if _, err := exec.LookPath("sudo"); err != nil {
		t.Skip("sudo not on PATH; escalation integration leg not exercisable here")
	}

	if err := exec.Command("sudo", "-n", "true").Run(); err != nil {
		t.Skipf("sudo present but non-interactive escalation is unavailable: %v", err)
	}
	r, err := sysexec.NewRunner(sysexec.Sudo)
	if err != nil {
		t.Fatalf("NewRunner(Sudo): %v", err)
	}
	return r
}

func TestRunner_EscalatedRunsAsRoot_Integration(t *testing.T) {
	res, err := sudoRunner(t).Run(context.Background(), sysexec.Command{Name: "id", Args: []string{"-u"}, Escalate: true})
	if err != nil {
		t.Fatalf("escalated Run err = %v", err)
	}
	if got := strings.TrimSpace(res.Stdout); got != "0" {
		t.Errorf("escalated id -u = %q, want 0 (root)", got)
	}
}

func TestRunner_EscalatedStdin_Integration(t *testing.T) {
	res, err := sudoRunner(t).Run(context.Background(), sysexec.Command{
		Name: "cat", Escalate: true, Stdin: strings.NewReader("escalated-input"),
	})
	if err != nil {
		t.Fatalf("escalated stdin Run err = %v", err)
	}
	if !strings.Contains(res.Stdout, "escalated-input") {
		t.Errorf("escalated cat stdout = %q, want the piped input", res.Stdout)
	}
}

func TestRunner_EscalatedStreaming_Integration(t *testing.T) {
	var lines []string
	res, err := sudoRunner(t).Stream(context.Background(),
		sysexec.Command{Name: "sh", Args: []string{"-c", "printf 'line1\\nline2\\nline3\\n'"}, Escalate: true},
		func(s sysexec.StreamType, line string, _ int64) {
			if s == sysexec.StreamStdout {
				lines = append(lines, strings.TrimRight(line, "\n"))
			}
		})
	if err != nil {
		t.Fatalf("escalated Stream err = %v", err)
	}
	if res.ExitCode != 0 {
		t.Errorf("exit = %d (stderr=%q), want 0", res.ExitCode, res.Stderr)
	}
	if strings.Join(lines, ",") != "line1,line2,line3" {
		t.Errorf("streamed lines = %v, want [line1 line2 line3]", lines)
	}
}

func TestRunner_EscalatedCommandNotFound_Integration(t *testing.T) {
	_, err := sudoRunner(t).Run(context.Background(), sysexec.Command{Name: "nonexistent-command-12345", Escalate: true})
	if err == nil {
		t.Fatal("expected an error for a nonexistent escalated command")
	}
	if !strings.Contains(err.Error(), "command not found") {
		t.Errorf("err = %v, want a command-not-found failure", err)
	}
}
