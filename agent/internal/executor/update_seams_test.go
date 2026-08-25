package executor

import (
	"context"
	"errors"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
)

func installShutdownStub(t *testing.T, exitCode int) (argvLog string) {
	t.Helper()
	stubDir := t.TempDir()
	argvLog = filepath.Join(stubDir, "argv")

	quotedArgvLog := "'" + strings.ReplaceAll(argvLog, "'", `'\''`) + "'"
	stub := fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' \"$@\" > %s\nexit %d\n", quotedArgvLog, exitCode)
	if err := os.WriteFile(filepath.Join(stubDir, "shutdown"), []byte(stub), 0o755); err != nil {
		t.Fatalf("write shutdown stub: %v", err)
	}
	t.Setenv("PATH", stubDir)

	p, err := osexec.LookPath("shutdown")
	if err != nil || filepath.Dir(p) != stubDir {
		t.Fatalf("refusing to run: `shutdown` must resolve inside the stub dir, got %q (%v)", p, err)
	}
	return argvLog
}

func newRebootExecutor(t *testing.T) *Executor {
	t.Helper()
	r, err := sysexec.NewRunner(sysexec.Direct)
	if err != nil {
		t.Fatalf("build direct runner: %v", err)
	}
	e := NewExecutor(r)
	return e
}

type countingNotify struct{ count *int }

func (n countingNotify) NotifyAll(context.Context, string, string) error             { *n.count++; return nil }
func (n countingNotify) NotifyUsers(context.Context, []string, string, string) error { return nil }

func TestScheduleRebootAfterUpdate(t *testing.T) {
	t.Run("schedules the reboot and notifies on success", func(t *testing.T) {
		argvLog := installShutdownStub(t, 0)
		notified := 0
		e := newRebootExecutor(t)
		e.deps.notify = countingNotify{count: &notified}

		var out strings.Builder
		if err := e.scheduleRebootAfterUpdate(context.Background(), &out); err != nil {
			t.Fatalf("scheduleRebootAfterUpdate = %v, want a scheduled reboot", err)
		}
		if notified != 1 {
			t.Errorf("users must be notified once when the reboot is scheduled, got %d", notified)
		}
		if !strings.Contains(out.String(), "Scheduled reboot") {
			t.Errorf("output = %q, want the scheduled-reboot line", out.String())
		}

		argv, err := os.ReadFile(argvLog)
		if err != nil {
			t.Fatalf("read stub argv: %v", err)
		}

		args := strings.Split(strings.TrimSuffix(string(argv), "\n"), "\n")
		wantArgs := []string{"-r", "+1", "System update requires reboot"}
		if !reflect.DeepEqual(args, wantArgs) {
			t.Errorf("shutdown argv = %q, want exactly %q", args, wantArgs)
		}
	})

	t.Run("schedule failure returns an error and suppresses notify", func(t *testing.T) {
		installShutdownStub(t, 1)
		notified := 0
		e := newRebootExecutor(t)
		e.deps.notify = countingNotify{count: &notified}

		var out strings.Builder
		err := e.scheduleRebootAfterUpdate(context.Background(), &out)
		if err == nil {
			t.Fatal("a failed reboot schedule must return an error, not a clean success")
		}
		if !strings.Contains(err.Error(), "schedule reboot") {
			t.Errorf("error = %q, want it to name the reboot scheduling failure", err)
		}
		if !strings.Contains(out.String(), "FAILED to schedule reboot") {
			t.Errorf("output = %q, want the FAILED line", out.String())
		}
		if notified != 0 {
			t.Errorf("users must NOT be notified when the reboot did not go out, got %d notifications", notified)
		}
	})

	t.Run("reboot failure joins with a prior error rather than demoting it", func(t *testing.T) {
		installShutdownStub(t, 1)

		var out strings.Builder
		prior := errors.New("apt upgrade failed")
		e := newRebootExecutor(t)
		e.deps.notify = countingNotify{}
		joined := errors.Join(prior, e.scheduleRebootAfterUpdate(context.Background(), &out))
		if !errors.Is(joined, prior) {
			t.Error("a prior upgrade error must stay visible alongside the reboot failure")
		}
		if !strings.Contains(joined.Error(), "schedule reboot") {
			t.Error("the reboot failure must not be demoted by a first-error-wins guard")
		}
	})

	t.Run("fails closed without a privilege runner", func(t *testing.T) {

		notified := 0

		var out strings.Builder
		e := &Executor{}
		err := e.scheduleRebootAfterUpdate(context.Background(), &out)
		if err == nil {
			t.Fatal("a reboot with no privilege runner must fail closed, not fall through to the global Direct runner")
		}
		if notified != 0 {
			t.Errorf("users must NOT be notified when no reboot can be scheduled, got %d notifications", notified)
		}
	})
}
