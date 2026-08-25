package executor

import (
	"context"
	"errors"
	"strings"
	"testing"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	sysfs "github.com/manchtools/cadestro/sdk/sys/fs"
	sysuser "github.com/manchtools/cadestro/sdk/sys/user"
)

type fakeExistsFS struct {
	sysfs.Manager
	ok          bool
	err         error
	calledPaths []string
}

func (f *fakeExistsFS) Exists(_ context.Context, path string) (bool, error) {
	f.calledPaths = append(f.calledPaths, path)
	return f.ok, f.err
}

type fakeEnsureHomeUser struct {
	sysuser.Manager
	calls []ensureHomeCall
	err   error
}

type ensureHomeCall struct {
	name string
	opts sysuser.EnsureHomeOptions
}

func (u *fakeEnsureHomeUser) EnsureHome(_ context.Context, name string, opts sysuser.EnsureHomeOptions) error {
	u.calls = append(u.calls, ensureHomeCall{name: name, opts: opts})
	return u.err
}

func swapHomeMgrs(t *testing.T, e *Executor, fs *fakeExistsFS, usr *fakeEnsureHomeUser) {
	t.Helper()
	e.deps.fs = fs
	e.deps.user = usr
}

func TestEnsureHomeIfMissing_ProbeErrorFailsClosed(t *testing.T) {
	fs := &fakeExistsFS{err: errors.New("permission denied")}
	usr := &fakeEnsureHomeUser{}
	e := NewExecutor(nil)
	swapHomeMgrs(t, e, fs, usr)

	var out strings.Builder
	changed := e.ensureHomeIfMissing(context.Background(),
		&pb.UserParams{Username: "alice", CreateHome: true}, "", &out)

	if changed {
		t.Error("changed=true on an indeterminate probe; must report no change")
	}
	if len(usr.calls) != 0 {
		t.Fatalf("EnsureHome called %d time(s) on a probe error; must fail closed and skip", len(usr.calls))
	}
	if !strings.Contains(out.String(), "could not check home directory") {
		t.Errorf("probe error not surfaced; output = %q", out.String())
	}
}

func TestEnsureHomeIfMissing_MissingCreatesWithOwnershipAndMode(t *testing.T) {
	fs := &fakeExistsFS{ok: false}
	usr := &fakeEnsureHomeUser{}
	e := NewExecutor(nil)
	swapHomeMgrs(t, e, fs, usr)

	var out strings.Builder
	changed := e.ensureHomeIfMissing(context.Background(),
		&pb.UserParams{Username: "alice", PrimaryGroup: "staff", CreateHome: true}, "", &out)

	if !changed {
		t.Error("changed=false though a missing home was created")
	}
	if len(usr.calls) != 1 {
		t.Fatalf("expected exactly 1 EnsureHome call, got %d", len(usr.calls))
	}
	c := usr.calls[0]
	if c.name != "alice" {
		t.Errorf("EnsureHome name = %q, want alice", c.name)
	}
	if c.opts.Group != "staff" {
		t.Errorf("EnsureHome group = %q, want staff (homeGroupFor)", c.opts.Group)
	}
	if c.opts.Mode != 0o700 {
		t.Errorf("EnsureHome mode = %o, want 0700", c.opts.Mode)
	}
	if !strings.Contains(out.String(), "created missing home directory") {
		t.Errorf("creation not surfaced; output = %q", out.String())
	}
}

func TestEnsureHomeIfMissing_PresentIsIdempotent(t *testing.T) {
	fs := &fakeExistsFS{ok: true}
	usr := &fakeEnsureHomeUser{}
	e := NewExecutor(nil)
	swapHomeMgrs(t, e, fs, usr)

	var out strings.Builder
	changed := e.ensureHomeIfMissing(context.Background(),
		&pb.UserParams{Username: "alice", CreateHome: true}, "", &out)

	if changed {
		t.Error("changed=true though the home already existed")
	}
	if len(usr.calls) != 0 {
		t.Errorf("EnsureHome called %d time(s) on a present home", len(usr.calls))
	}
}

func TestEnsureHomeIfMissing_NoCreateHomeSkipsProbe(t *testing.T) {
	fs := &fakeExistsFS{ok: false}
	usr := &fakeEnsureHomeUser{}
	e := NewExecutor(nil)
	swapHomeMgrs(t, e, fs, usr)

	var out strings.Builder
	changed := e.ensureHomeIfMissing(context.Background(),
		&pb.UserParams{Username: "alice", CreateHome: false}, "", &out)

	if changed {
		t.Error("changed=true with create_home=false")
	}
	if len(fs.calledPaths) != 0 {
		t.Errorf("probed Exists %d time(s) with create_home=false; must skip entirely", len(fs.calledPaths))
	}
	if len(usr.calls) != 0 {
		t.Errorf("EnsureHome called with create_home=false")
	}
}

func TestEnsureHomeIfMissing_HomeDirResolution(t *testing.T) {
	cases := []struct {
		name        string
		homeDir     string
		currentHome string
		want        string
	}{
		{"explicit", "/srv/alice", "/home/alice", "/srv/alice"},
		{"passwd-fallback", "", "/var/home/alice", "/var/home/alice"},
		{"default", "", "", "/home/alice"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fs := &fakeExistsFS{ok: true}
			usr := &fakeEnsureHomeUser{}
			e := NewExecutor(nil)
			swapHomeMgrs(t, e, fs, usr)

			var out strings.Builder
			e.ensureHomeIfMissing(context.Background(),
				&pb.UserParams{Username: "alice", HomeDir: tc.homeDir, CreateHome: true}, tc.currentHome, &out)

			if len(fs.calledPaths) != 1 || fs.calledPaths[0] != tc.want {
				t.Errorf("probed %v, want exactly [%s]", fs.calledPaths, tc.want)
			}
		})
	}
}
