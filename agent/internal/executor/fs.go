package executor

import (
	"context"
	"fmt"
	"os"
	"strconv"

	sysfs "github.com/manchtools/cadestro/sdk/sys/fs"
)

func getFileOwnership(path string) (owner, group string) {
	return sysfs.GetOwnership(path)
}

func (e *Executor) atomicWriteFile(ctx context.Context, path, content, mode, owner, group string) error {
	opts := sysfs.WriteOptions{Owner: owner, Group: group}
	if mode != "" {
		v, err := strconv.ParseUint(mode, 8, 32)
		if err != nil {
			return fmt.Errorf("invalid file mode %q: %w", mode, err)
		}
		opts.Mode = os.FileMode(v)
	}
	return e.deps.fs.WriteFile(ctx, path, []byte(content), opts)
}

func (e *Executor) readFileWithSudo(ctx context.Context, path string) (string, error) {
	b, err := e.deps.fs.ReadFile(ctx, path)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (e *Executor) fileExistsWithSudo(ctx context.Context, path string) bool {
	ok, _ := e.deps.fs.Exists(ctx, path)
	return ok
}

func statFile(ctx context.Context, path string) (os.FileMode, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return 0, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return 0, fmt.Errorf("statFile: refusing to follow symlink: %s", path)
	}
	return info.Mode(), nil
}

func (e *Executor) removeFileStrict(ctx context.Context, path string) error {
	return e.deps.fs.Remove(ctx, path)
}

func (e *Executor) createDirectory(ctx context.Context, path string, recursive bool) error {
	return e.deps.fs.Mkdir(ctx, path, sysfs.MkdirOptions{Recursive: recursive})
}

func (e *Executor) createDirectoryWithPermissions(ctx context.Context, path, mode, owner, group string, recursive bool) error {
	if err := e.deps.fs.Mkdir(ctx, path, sysfs.MkdirOptions{Recursive: recursive}); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	if mode == "" && owner == "" && group == "" {
		return nil
	}
	uid, gid := -1, -1
	if owner != "" || group != "" {
		var err error
		uid, gid, err = sysfs.ResolveOwnership(owner, group)
		if err != nil {
			return err
		}
	}
	perm := os.FileMode(0o755)
	if mode != "" {
		v, err := strconv.ParseUint(mode, 8, 32)
		if err != nil {
			return fmt.Errorf("invalid directory mode %q: %w", mode, err)
		}
		perm = os.FileMode(v)
	}
	return sysfs.SetDirPermissionsNoFollow(path, perm, uid, gid)
}

func (e *Executor) removeDirectory(ctx context.Context, path string) error {
	return e.deps.fs.RemoveDir(ctx, path)
}

func (e *Executor) userExists(ctx context.Context, username string) (bool, error) {
	return e.deps.user.Exists(ctx, username)
}

func (e *Executor) groupExists(ctx context.Context, groupName string) (bool, error) {
	return e.deps.user.GroupExists(ctx, groupName)
}
