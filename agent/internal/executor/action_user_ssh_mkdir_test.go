package executor

import (
	"context"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	sysfs "github.com/manchtools/cadestro/sdk/sys/fs"
)

type recordingSSHFS struct {
	sysfs.Manager
	mkdirCalls []sshMkdirCall
}

type sshMkdirCall struct {
	path string
	opts sysfs.MkdirOptions
}

func (f *recordingSSHFS) ReadFile(_ context.Context, path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, os.ErrNotExist
		}
		return nil, err
	}
	return b, nil
}

func (f *recordingSSHFS) Mkdir(_ context.Context, path string, opts sysfs.MkdirOptions) error {
	f.mkdirCalls = append(f.mkdirCalls, sshMkdirCall{path: path, opts: opts})
	if opts.Recursive {
		return os.MkdirAll(path, 0o700)
	}
	return os.Mkdir(path, 0o700)
}

func (f *recordingSSHFS) WriteFile(_ context.Context, path string, data []byte, opts sysfs.WriteOptions) error {
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	if opts.Mode != 0 {
		return os.Chmod(path, opts.Mode)
	}
	return nil
}

func TestSetupSSHKeys_CreatesDotSSHViaFSManager(t *testing.T) {
	cur, err := user.Current()
	if err != nil {
		t.Skipf("cannot resolve current user: %v", err)
	}
	gid, err := strconv.Atoi(cur.Gid)
	if err != nil {
		t.Skipf("non-numeric primary gid %q: %v", cur.Gid, err)
	}

	fake := &recordingSSHFS{}

	home := t.TempDir()
	const keyLine = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAtestkey user@host"
	params := &pb.UserParams{
		Username:          cur.Username,
		Gid:               int32(gid),
		HomeDir:           home,
		SshAuthorizedKeys: []string{keyLine},
	}

	e := NewExecutor(nil)
	e.deps.fs = fake
	var out strings.Builder
	changed, err := e.setupSSHKeys(context.Background(), params, &out)
	if err != nil {
		t.Fatalf("setupSSHKeys: %v", err)
	}
	if !changed {
		t.Error("expected changed=true on first SSH key setup")
	}

	if len(fake.mkdirCalls) != 1 {
		t.Fatalf("expected exactly 1 fsMgr.Mkdir call, got %d (raw sudo mkdir not replaced?)", len(fake.mkdirCalls))
	}
	mc := fake.mkdirCalls[0]
	wantDir := filepath.Join(home, ".ssh")
	if mc.path != wantDir {
		t.Errorf("Mkdir path = %q, want %q", mc.path, wantDir)
	}
	if !mc.opts.Recursive {
		t.Error("Mkdir must be recursive (mkdir -p equivalent)")
	}
	if mc.opts.Mode != 0 {
		t.Errorf("Mkdir opts.Mode = %o, want 0 — perms must be set via the O_NOFOLLOW fd, not a symlink-following path chmod", mc.opts.Mode)
	}

	if got, rerr := os.ReadFile(filepath.Join(wantDir, "authorized_keys")); rerr != nil {
		t.Fatalf("read authorized_keys: %v", rerr)
	} else if !strings.Contains(string(got), keyLine) {
		t.Errorf("authorized_keys = %q, want it to contain the configured key", got)
	}
	if fi, serr := os.Stat(wantDir); serr != nil {
		t.Fatalf("stat .ssh: %v", serr)
	} else if fi.Mode().Perm() != 0o700 {
		t.Errorf(".ssh perm = %o, want 0700", fi.Mode().Perm())
	}
}
