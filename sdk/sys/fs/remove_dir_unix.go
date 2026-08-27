//go:build unix

package fs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

func removeDirSecure(ctx context.Context, path string) error {
	parent := filepath.Dir(path)
	base := filepath.Base(path)

	pfd, err := openNoFollowChain(parent)
	if err != nil {
		return err
	}
	defer func() { _ = unix.Close(pfd) }()

	var st unix.Stat_t
	if err := unix.Fstatat(pfd, base, &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	switch st.Mode & unix.S_IFMT {
	case unix.S_IFLNK:
		return fmt.Errorf("refusing to remove symlink %s", path)
	case unix.S_IFDIR:

	default:
		return fmt.Errorf("refusing to remove non-directory %s", path)
	}

	return removeAtRecursive(ctx, pfd, base)
}

func openNoFollowChain(dir string) (int, error) {
	clean := filepath.Clean(dir)

	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, fmt.Errorf("open /: %w", err)
	}
	if clean == "/" {
		return fd, nil
	}

	rest := strings.TrimPrefix(clean, "/")
	for _, comp := range strings.Split(rest, "/") {
		if comp == "" || comp == "." {
			continue
		}
		next, openErr := unix.Openat(fd, comp, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		_ = unix.Close(fd)
		if openErr != nil {
			return -1, fmt.Errorf("open component %q of %s without following symlinks: %w", comp, dir, openErr)
		}
		fd = next
	}
	return fd, nil
}

func removeAtRecursive(ctx context.Context, dirfd int, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	var st unix.Stat_t
	if err := unix.Fstatat(dirfd, name, &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return fmt.Errorf("stat %s: %w", name, err)
	}
	if st.Mode&unix.S_IFMT != unix.S_IFDIR {

		if err := unix.Unlinkat(dirfd, name, 0); err != nil {
			return fmt.Errorf("unlink %s: %w", name, err)
		}
		return nil
	}

	cfd, err := unix.Openat(dirfd, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("open dir %s: %w", name, err)
	}

	f := os.NewFile(uintptr(cfd), name)
	children, readErr := f.Readdirnames(-1)
	if readErr != nil {
		_ = f.Close()
		return fmt.Errorf("read dir %s: %w", name, readErr)
	}
	for _, child := range children {
		if child == "." || child == ".." {
			continue
		}
		if err := removeAtRecursive(ctx, cfd, child); err != nil {
			_ = f.Close()
			return err
		}
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close dir %s: %w", name, err)
	}
	if err := unix.Unlinkat(dirfd, name, unix.AT_REMOVEDIR); err != nil {
		return fmt.Errorf("rmdir %s: %w", name, err)
	}
	return nil
}
