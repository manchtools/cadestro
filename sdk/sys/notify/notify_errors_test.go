package notify

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/manchtools/cadestro/sdk/sys/exec"
	"github.com/manchtools/cadestro/sdk/sys/exec/exectest"
)

func seamNotifySendAbsent(t *testing.T) {
	t.Helper()
	ol, os_ := lookPath, statSocket
	t.Cleanup(func() { lookPath, statSocket = ol, os_ })
	lookPath = func(string) (string, error) { return "", errors.New("not found") }
	statSocket = func(string) (os.FileInfo, error) { return nil, errors.New("no socket") }
}

func TestNotifyAll_ReturnsErrorOnWallFailure(t *testing.T) {
	seamNotifySendAbsent(t)
	r := exectest.New(exec.Sudo)
	r.Push(exec.Result{ExitCode: 1, Stderr: "wall: permission denied"}, nil)
	if err := mgr(t, r).NotifyAll(context.Background(), "T", "m"); err == nil {
		t.Error("NotifyAll = nil when wall failed; the failure must be surfaced, not swallowed")
	}
}

func TestNotifyUsers_ReturnsErrorOnWallFailure(t *testing.T) {
	seamNotifySendAbsent(t)
	r := exectest.New(exec.Sudo)
	r.Push(exec.Result{}, errors.New("runner down"))
	if err := mgr(t, r).NotifyUsers(context.Background(), []string{"alice"}, "T", "m"); err == nil {
		t.Error("NotifyUsers = nil when wall failed")
	}
}

func TestNotifyAll_NilOnSuccessNoDesktopTool(t *testing.T) {
	seamNotifySendAbsent(t)
	r := exectest.New(exec.Sudo)
	r.Push(exec.Result{}, nil)
	if err := mgr(t, r).NotifyAll(context.Background(), "T", "m"); err != nil {
		t.Errorf("NotifyAll = %v, want nil (wall ok, notify-send absent is a graceful skip)", err)
	}
}

func TestNotifyAll_AggregatesDesktopDeliveryFailure(t *testing.T) {
	seamPresent(t)
	r := exectest.New(exec.Sudo)
	r.Push(exec.Result{}, nil)
	r.Push(exec.Result{Stdout: "c1 1000 alice seat0 -"}, nil)
	r.Push(exec.Result{Stdout: "Type=wayland\nName=alice\nUser=1000"}, nil)
	r.Push(exec.Result{ExitCode: 1, Stderr: "notify-send failed"}, nil)
	if err := mgr(t, r).NotifyAll(context.Background(), "T", "m"); err == nil {
		t.Error("NotifyAll = nil when a desktop notification delivery failed")
	}
}

func TestNotifyAll_NilOnFullSuccess(t *testing.T) {
	seamPresent(t)
	r := exectest.New(exec.Sudo)
	r.Push(exec.Result{}, nil)
	r.Push(exec.Result{Stdout: "c1 1000 alice seat0 -"}, nil)
	r.Push(exec.Result{Stdout: "Type=wayland\nName=alice\nUser=1000"}, nil)
	r.Push(exec.Result{}, nil)
	if err := mgr(t, r).NotifyAll(context.Background(), "T", "m"); err != nil {
		t.Errorf("NotifyAll = %v, want nil on full success", err)
	}
}

func TestNotifyAll_ErrorWhenSessionListFails(t *testing.T) {
	seamPresent(t)
	r := exectest.New(exec.Sudo)
	r.Push(exec.Result{}, nil)
	r.Push(exec.Result{ExitCode: 1, Stderr: "loginctl down"}, nil)
	if err := mgr(t, r).NotifyAll(context.Background(), "T", "m"); err == nil {
		t.Error("NotifyAll = nil when session enumeration failed")
	}
}

func TestNotifyAll_MissingDBusSocketIsGracefulSkip(t *testing.T) {
	ol, os_ := lookPath, statSocket
	t.Cleanup(func() { lookPath, statSocket = ol, os_ })
	lookPath = func(string) (string, error) { return "/usr/bin/notify-send", nil }
	statSocket = func(string) (os.FileInfo, error) { return nil, errors.New("no socket") }
	r := exectest.New(exec.Sudo)
	r.Push(exec.Result{}, nil)
	r.Push(exec.Result{Stdout: "c1 1000 alice seat0 -"}, nil)
	r.Push(exec.Result{Stdout: "Type=wayland\nName=alice\nUser=1000"}, nil)
	if err := mgr(t, r).NotifyAll(context.Background(), "T", "m"); err != nil {
		t.Errorf("NotifyAll = %v, want nil (missing D-Bus socket is a graceful skip)", err)
	}

	for _, c := range r.Calls() {
		if c.Name == "env" {
			t.Error("notify-send ran despite a missing D-Bus socket")
		}
	}
}

func TestNotifyAll_ErrorWhenSessionListRunnerErrors(t *testing.T) {
	seamPresent(t)
	r := exectest.New(exec.Sudo)
	r.Push(exec.Result{}, nil)
	r.Push(exec.Result{}, errors.New("runner down"))
	if err := mgr(t, r).NotifyAll(context.Background(), "T", "m"); err == nil {
		t.Error("NotifyAll = nil when the session-list runner errored")
	}
}

func TestNotifyAll_ErrorWhenDeliveryRunnerErrors(t *testing.T) {
	seamPresent(t)
	r := exectest.New(exec.Sudo)
	r.Push(exec.Result{}, nil)
	r.Push(exec.Result{Stdout: "c1 1000 alice seat0 -"}, nil)
	r.Push(exec.Result{Stdout: "Type=wayland\nName=alice\nUser=1000"}, nil)
	r.Push(exec.Result{}, errors.New("runner down"))
	if err := mgr(t, r).NotifyAll(context.Background(), "T", "m"); err == nil {
		t.Error("NotifyAll = nil when a desktop delivery runner errored")
	}
}
