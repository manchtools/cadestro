package executor

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	sysuser "github.com/manchtools/cadestro/sdk/sys/user"
)

type fakeRootDisableUser struct {
	sysuser.Manager
	info     sysuser.Info
	locked   []string
	modified []sysuser.ModifyOptions
}

func (f *fakeRootDisableUser) Get(context.Context, string) (sysuser.Info, error) {
	return f.info, nil
}
func (f *fakeRootDisableUser) Lock(_ context.Context, name string) error {
	f.locked = append(f.locked, name)
	return nil
}
func (f *fakeRootDisableUser) Modify(_ context.Context, _ string, opts sysuser.ModifyOptions) error {
	f.modified = append(f.modified, opts)
	return nil
}

func swapUserMgr(t *testing.T, e *Executor, m sysuser.Manager) {
	t.Helper()
	e.deps.user = m
}

func TestUpdateUser_DisableRoot_LockOnlyKeepsShell(t *testing.T) {
	fake := &fakeRootDisableUser{info: sysuser.Info{UID: 0, Shell: "/bin/bash", Locked: false}}
	var logBuf bytes.Buffer
	e := NewExecutor(nil)
	e.deps.user = fake
	e.logger = slog.New(slog.NewTextHandler(&logBuf, nil))
	e.now = time.Now

	var out strings.Builder
	_, changed, err := e.updateUser(context.Background(), &pb.UserParams{
		Username: "root",
		Disabled: true,
	}, &out)
	if err != nil {
		t.Fatalf("updateUser: %v", err)
	}
	if !changed {
		t.Fatal("locking root must report changed")
	}
	if len(fake.locked) != 1 || fake.locked[0] != "root" {
		t.Fatalf("root must be locked exactly once, got %v", fake.locked)
	}
	for _, m := range fake.modified {
		if m.Shell != "" {
			t.Fatalf("disabling root must not touch the shell (lock-only), got Modify(Shell=%q)", m.Shell)
		}
	}
	if !strings.Contains(logBuf.String(), "level=WARN") || !strings.Contains(logBuf.String(), "root") {
		t.Fatalf("locking root must warn loudly in the journal, got: %s", logBuf.String())
	}
}

func TestUpdateUser_DisableRegularUser_StillDefaultsNologin(t *testing.T) {
	fake := &fakeRootDisableUser{info: sysuser.Info{UID: 1000, Shell: "/bin/bash", Locked: false}}
	e := NewExecutor(nil)
	swapUserMgr(t, e, fake)
	e.logger = slog.Default()
	e.now = time.Now
	var out strings.Builder
	_, _, err := e.updateUser(context.Background(), &pb.UserParams{
		Username: "alice",
		Disabled: true,
	}, &out)
	if err != nil {
		t.Fatalf("updateUser: %v", err)
	}
	found := false
	for _, m := range fake.modified {
		if m.Shell == "/usr/sbin/nologin" {
			found = true
		}
	}
	if !found {
		t.Fatalf("a disabled regular user must still default to the nologin shell, got %v", fake.modified)
	}
	if len(fake.locked) != 1 || fake.locked[0] != "alice" {
		t.Fatalf("alice must be locked, got %v", fake.locked)
	}
}

func TestUpdateUser_DisableRoot_ExplicitShellHonored(t *testing.T) {
	fake := &fakeRootDisableUser{info: sysuser.Info{UID: 0, Shell: "/bin/bash", Locked: false}}
	e := NewExecutor(nil)
	swapUserMgr(t, e, fake)
	e.logger = slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	e.now = time.Now
	var out strings.Builder
	_, _, err := e.updateUser(context.Background(), &pb.UserParams{
		Username: "root",
		Disabled: true,
		Shell:    "/usr/sbin/nologin",
	}, &out)
	if err != nil {
		t.Fatalf("updateUser: %v", err)
	}
	found := false
	for _, m := range fake.modified {
		if m.Shell == "/usr/sbin/nologin" {
			found = true
		}
	}
	if !found {
		t.Fatal("an explicitly requested shell must still be applied to root")
	}
}

func TestUpdateUser_DisableRenamedSuperuser_LockOnlyKeepsShell(t *testing.T) {
	fake := &fakeRootDisableUser{info: sysuser.Info{UID: 0, Shell: "/bin/bash", Locked: false}}
	var logBuf bytes.Buffer
	e := NewExecutor(nil)
	swapUserMgr(t, e, fake)
	e.logger = slog.New(slog.NewTextHandler(&logBuf, nil))
	e.now = time.Now

	var out strings.Builder
	_, changed, err := e.updateUser(context.Background(), &pb.UserParams{
		Username: "sysadm",
		Disabled: true,
	}, &out)
	if err != nil {
		t.Fatalf("updateUser: %v", err)
	}
	if !changed {
		t.Fatal("locking the superuser must report changed")
	}
	for _, m := range fake.modified {
		if m.Shell != "" {
			t.Fatalf("disabling a UID-0 account must not touch the shell (lock-only), got Modify(Shell=%q)", m.Shell)
		}
	}
	if !strings.Contains(logBuf.String(), "level=WARN") || !strings.Contains(logBuf.String(), "sysadm") {
		t.Fatalf("locking a UID-0 account must warn loudly in the journal, got: %s", logBuf.String())
	}
}
