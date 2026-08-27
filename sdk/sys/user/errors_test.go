package user

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/manchtools/cadestro/sdk/sys/exec"
	"github.com/manchtools/cadestro/sdk/sys/exec/exectest"
)

func TestRun_PropagatesRunnerError(t *testing.T) {
	f := exectest.New(exec.Sudo)
	f.Push(exec.Result{}, exec.ErrEscalationUnavailable)
	if err := mgr(t, f).Lock(context.Background(), "deploy"); !errors.Is(err, exec.ErrEscalationUnavailable) {
		t.Errorf("Lock err = %v, want ErrEscalationUnavailable", err)
	}
}

func TestQuery_PropagatesRunnerError(t *testing.T) {
	f := exectest.New(exec.Sudo)
	f.Push(exec.Result{}, exec.ErrEscalationUnavailable)
	if _, err := mgr(t, f).PrimaryGroup(context.Background(), "deploy"); !errors.Is(err, exec.ErrEscalationUnavailable) {
		t.Errorf("PrimaryGroup err = %v, want ErrEscalationUnavailable", err)
	}
}

func TestSetPassword_PropagatesRunnerError(t *testing.T) {
	f := exectest.New(exec.Sudo)
	f.Push(exec.Result{}, exec.ErrEscalationUnavailable)
	pw, _ := exec.NewSecret("TestPass123!")
	if err := mgr(t, f).SetPassword(context.Background(), "deploy", pw); !errors.Is(err, exec.ErrEscalationUnavailable) {
		t.Errorf("SetPassword err = %v, want ErrEscalationUnavailable", err)
	}
}

func TestExists_PropagatesRunnerError(t *testing.T) {
	f := exectest.New(exec.Sudo)
	f.Push(exec.Result{}, exec.ErrEscalationUnavailable)
	if _, err := mgr(t, f).Exists(context.Background(), "deploy"); !errors.Is(err, exec.ErrEscalationUnavailable) {
		t.Errorf("Exists err = %v, want ErrEscalationUnavailable", err)
	}
}

func TestGroupExists_PropagatesRunnerError(t *testing.T) {
	f := exectest.New(exec.Sudo)
	f.Push(exec.Result{}, exec.ErrEscalationUnavailable)
	if _, err := mgr(t, f).GroupExists(context.Background(), "staff"); !errors.Is(err, exec.ErrEscalationUnavailable) {
		t.Errorf("GroupExists err = %v, want ErrEscalationUnavailable", err)
	}
}

func TestGroupMembership_RejectsInvalidGroupName(t *testing.T) {
	f := exectest.New(exec.Direct)
	if err := mgr(t, f).AddToGroup(context.Background(), "deploy", "-G"); err == nil {
		t.Error("AddToGroup accepted a flag-shaped group name")
	}
	if err := mgr(t, f).RemoveFromGroup(context.Background(), "deploy", "-G"); err == nil {
		t.Error("RemoveFromGroup accepted a flag-shaped group name")
	}
	if len(f.Calls()) != 0 {
		t.Errorf("ran a command for an invalid group name: %+v", f.Calls())
	}
}

func TestQuery_HonorsCallerDeadline(t *testing.T) {
	f := exectest.New(exec.Direct)
	f.Push(exec.Result{Stdout: "staff\n"}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := mgr(t, f).PrimaryGroup(ctx, "deploy"); err != nil {
		t.Fatalf("PrimaryGroup with a deadline ctx: %v", err)
	}
}

func TestQuery_NonZeroExitIsCommandError(t *testing.T) {
	f := exectest.New(exec.Direct)
	f.Push(exec.Result{ExitCode: 1, Stderr: "id: 'ghost': no such user"}, nil)
	_, err := mgr(t, f).PrimaryGroup(context.Background(), "ghost")
	var ce *exec.CommandError
	if !errors.As(err, &ce) || ce.ExitCode != 1 {
		t.Errorf("PrimaryGroup err = %v, want *exec.CommandError exit 1", err)
	}
}

func TestGet_PasswdLookupFailure(t *testing.T) {
	f := exectest.New(exec.Direct)
	f.Push(exec.Result{ExitCode: 2}, nil)
	if _, err := mgr(t, f).Get(context.Background(), "ghost"); err == nil {
		t.Error("Get returned nil error for an unknown user")
	}
}

func TestGroupMembers_AbsentGroupIsErrNotExist(t *testing.T) {
	f := exectest.New(exec.Direct)
	f.Push(exec.Result{ExitCode: 2}, nil)
	members, err := mgr(t, f).GroupMembers(context.Background(), "ghosts")
	if !errors.Is(err, os.ErrNotExist) || members != nil {
		t.Errorf("GroupMembers(absent) = (%v,%v), want (nil, ErrNotExist)", members, err)
	}
}

func TestGroupMembers_RealErrorPropagates(t *testing.T) {
	f := exectest.New(exec.Direct)
	f.Push(exec.Result{}, exec.ErrEscalationDenied)
	members, err := mgr(t, f).GroupMembers(context.Background(), "docker")
	if err == nil {
		t.Fatal("a getent failure that is not exit-2-not-found must propagate, not read as 'no members'")
	}
	if members != nil {
		t.Errorf("members = %v, want nil on a real error", members)
	}
}

func TestSupplementaryGroups_PrimaryListFailure(t *testing.T) {
	f := exectest.New(exec.Direct)
	f.Push(exec.Result{ExitCode: 1, Stderr: "id: 'ghost': no such user"}, nil)
	if _, err := mgr(t, f).SupplementaryGroups(context.Background(), "ghost"); err == nil {
		t.Error("SupplementaryGroups returned nil error when id -Gn failed")
	}
}

func TestSupplementaryGroups_PrimaryLookupFailsClosed(t *testing.T) {
	f := exectest.New(exec.Direct)
	f.Push(exec.Result{Stdout: "staff docker sudo\n"}, nil)
	f.Push(exec.Result{ExitCode: 1}, nil)
	groups, err := mgr(t, f).SupplementaryGroups(context.Background(), "deploy")
	if err == nil {
		t.Errorf("SupplementaryGroups returned (%v,nil); want a fail-closed error when the primary lookup fails", groups)
	}
	if groups != nil {
		t.Errorf("groups = %v, want nil on the fail-closed path", groups)
	}
}

func TestCreate_ChownFailureSurfaces(t *testing.T) {
	existing := t.TempDir()
	f := exectest.New(exec.Direct)
	f2 := newFakeFS()
	f2.chownErr = exec.ErrEscalationDenied
	f2.install(t)

	err := mgr(t, f).Create(context.Background(), "deploy", CreateOptions{HomeDir: existing, CreateHome: true})
	if err == nil || !strings.Contains(err.Error(), "ownership") {
		t.Errorf("Create err = %v, want an ownership-fix failure", err)
	}
}

func TestKillSessions_PkillExecError(t *testing.T) {
	f := exectest.New(exec.Direct)
	f.Push(exec.Result{ExitCode: 1}, nil)
	f.Push(exec.Result{}, exec.ErrEscalationUnavailable)
	err := mgr(t, f).KillSessions(context.Background(), "deploy")
	if !errors.Is(err, exec.ErrEscalationUnavailable) {
		t.Errorf("KillSessions err = %v, want the wrapped pkill exec error", err)
	}
}

func TestGet_ShadowUnreadableLeavesUnlocked(t *testing.T) {
	f := exectest.New(exec.Sudo)
	f.Push(exec.Result{Stdout: "deploy:x:1000:1000:Deploy User:/home/deploy:/bin/bash\n"}, nil)
	f.Push(exec.Result{Stdout: "deploy:x:1000:\n"}, nil)
	f.Push(exec.Result{Stdout: "deploy sudo\n"}, nil)
	f.Push(exec.Result{}, exec.ErrEscalationDenied)
	info, err := mgr(t, f).Get(context.Background(), "deploy")
	if err != nil {
		t.Fatalf("Get should tolerate an unreadable shadow: %v", err)
	}
	if info.Locked {
		t.Error("Locked = true, want false when shadow is unreadable")
	}
	if info.LockedKnown {
		t.Error("LockedKnown = true, want false when the shadow read failed — unknown must not look unlocked")
	}
}
