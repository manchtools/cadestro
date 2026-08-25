package user

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/manchtools/cadestro/sdk/sys/exec"
	"github.com/manchtools/cadestro/sdk/sys/exec/exectest"
)

const deployPasswd = "deploy:x:1001:1001:Deploy:/home/deploy:/bin/bash\n"

func TestEnsureHome_MissingCreatesSeedsOwnsAndModes(t *testing.T) {
	f := exectest.New(exec.Direct)
	f.Push(exec.Result{Stdout: deployPasswd}, nil)
	ffs := newFakeFS().install(t)
	ffs.present["/etc/skel"] = true

	if err := mgr(t, f).EnsureHome(context.Background(), "deploy", EnsureHomeOptions{Group: "deploy"}); err != nil {
		t.Fatal(err)
	}
	if len(ffs.mkdirs) != 1 || ffs.mkdirs[0] != "/home/deploy" {
		t.Errorf("mkdirs = %v, want [/home/deploy]", ffs.mkdirs)
	}
	if len(ffs.copies) != 1 || ffs.copies[0].src != "/etc/skel" || ffs.copies[0].dst != "/home/deploy" {
		t.Errorf("copies = %v, want one /etc/skel → /home/deploy", ffs.copies)
	}
	if !ffs.chown.called || ffs.chown.path != "/home/deploy" || ffs.chown.owner != "deploy" || ffs.chown.group != "deploy" {
		t.Errorf("chown = %+v, want recursive deploy:deploy on /home/deploy", ffs.chown)
	}
	if ffs.chmods.path != "/home/deploy" || ffs.chmods.mode != 0o700 {
		t.Errorf("chmod = %+v, want 0700 on the home root", ffs.chmods)
	}
}

func TestEnsureHome_ExistingDoesNotReseedButFixesOwnerAndMode(t *testing.T) {
	f := exectest.New(exec.Direct)
	f.Push(exec.Result{Stdout: deployPasswd}, nil)
	ffs := newFakeFS().install(t)
	ffs.present["/home/deploy"] = true
	ffs.present["/etc/skel"] = true

	if err := mgr(t, f).EnsureHome(context.Background(), "deploy", EnsureHomeOptions{Group: "staff", Mode: 0o711}); err != nil {
		t.Fatal(err)
	}
	if len(ffs.mkdirs) != 0 {
		t.Errorf("mkdirs = %v, want none (home already exists)", ffs.mkdirs)
	}
	if len(ffs.copies) != 0 {
		t.Errorf("copies = %v, want NO skel reseed over an existing home", ffs.copies)
	}
	if !ffs.chown.called || ffs.chown.group != "staff" {
		t.Errorf("chown = %+v, want recursive ownership with group staff", ffs.chown)
	}
	if ffs.chmods.mode != 0o711 {
		t.Errorf("chmod mode = %v, want the requested 0711", ffs.chmods.mode)
	}
}

func TestEnsureHome_DefaultsGroupToPrimary(t *testing.T) {
	f := exectest.New(exec.Direct)
	f.Push(exec.Result{Stdout: deployPasswd}, nil)
	f.Push(exec.Result{Stdout: "deploy:x:1001:\n"}, nil)
	f.Push(exec.Result{Stdout: "deploy\n"}, nil)
	f.Push(exec.Result{Stdout: "deploy:$6$h:19000:0:99999:7:::\n"}, nil)
	f.Push(exec.Result{Stdout: "devs\n"}, nil)
	ffs := newFakeFS().install(t)
	ffs.present["/home/deploy"] = true

	if err := mgr(t, f).EnsureHome(context.Background(), "deploy", EnsureHomeOptions{}); err != nil {
		t.Fatal(err)
	}
	if ffs.chown.group != "devs" {
		t.Errorf("chown group = %q, want the resolved primary group 'devs'", ffs.chown.group)
	}
}

func TestEnsureHome_NoSkelStillCreatesEmptyHome(t *testing.T) {
	f := exectest.New(exec.Direct)
	f.Push(exec.Result{Stdout: deployPasswd}, nil)
	ffs := newFakeFS().install(t)

	if err := mgr(t, f).EnsureHome(context.Background(), "deploy", EnsureHomeOptions{Group: "deploy"}); err != nil {
		t.Fatal(err)
	}
	if len(ffs.mkdirs) != 1 {
		t.Errorf("mkdirs = %v, want the home created even without skel", ffs.mkdirs)
	}
	if len(ffs.copies) != 0 {
		t.Errorf("copies = %v, want no copy when skel is absent", ffs.copies)
	}
}

func TestEnsureHome_UserNotFoundErrors(t *testing.T) {
	f := exectest.New(exec.Direct)
	f.Push(exec.Result{ExitCode: 2}, nil)
	ffs := newFakeFS().install(t)

	if err := mgr(t, f).EnsureHome(context.Background(), "ghost", EnsureHomeOptions{}); err == nil {
		t.Fatal("EnsureHome on a nonexistent user must error")
	}
	if len(ffs.mkdirs) != 0 || len(ffs.copies) != 0 || ffs.chown.called {
		t.Error("EnsureHome touched the filesystem for a nonexistent user")
	}
}

func TestEnsureHome_MkdirFailureAborts(t *testing.T) {
	f := exectest.New(exec.Direct)
	f.Push(exec.Result{Stdout: deployPasswd}, nil)
	ffs := newFakeFS().install(t)
	ffs.present["/etc/skel"] = true
	ffs.mkdirErr = errors.New("read-only fs")

	err := mgr(t, f).EnsureHome(context.Background(), "deploy", EnsureHomeOptions{Group: "deploy"})
	if err == nil || !strings.Contains(err.Error(), "create") {
		t.Fatalf("err = %v, want a wrapped create failure", err)
	}
	if len(ffs.copies) != 0 || ffs.chown.called || ffs.chmods.path != "" {
		t.Errorf("EnsureHome seeded/owned/chmod'd after a failed mkdir (copies=%v chown=%v chmod=%q)",
			ffs.copies, ffs.chown.called, ffs.chmods.path)
	}
}

func TestEnsureHome_InvalidUsernameRejectedBeforeExec(t *testing.T) {
	f := exectest.New(exec.Direct)
	newFakeFS().install(t)
	if err := mgr(t, f).EnsureHome(context.Background(), "-rf", EnsureHomeOptions{}); err == nil {
		t.Fatal("a flag-shaped username must be rejected")
	}
	if len(f.Calls()) != 0 {
		t.Errorf("a flag-shaped username reached exec (%d calls)", len(f.Calls()))
	}
}
