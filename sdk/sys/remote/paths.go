package remote

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	sysfs "github.com/manchtools/cadestro/sdk/sys/fs"
)

var wipeAllowedRoots = []string{
	"/var/lib/cadestro/",
	"/etc/cadestro/",
}

func isManagedRoot(clean string) bool {
	for _, root := range wipeAllowedRoots {

		root = strings.TrimSuffix(root, "/") + "/"
		if strings.HasPrefix(clean+"/", root) {
			return true
		}
	}
	return false
}

var (
	recordedDestsMu sync.RWMutex
	recordedDests   = make(map[string]struct{})
)

func validateDestination(path string) error {
	trim := strings.TrimSpace(path)
	if trim == "" {
		return fmt.Errorf("%w: empty path", ErrUnsafeDestination)
	}
	if !filepath.IsAbs(trim) {
		return fmt.Errorf("%w: %s is not absolute", ErrUnsafeDestination, path)
	}
	clean := filepath.Clean(trim)
	if clean == "/" {
		return fmt.Errorf("%w: %s resolves to /", ErrUnsafeDestination, path)
	}

	if isManagedRoot(clean) {
		return nil
	}

	if sysfs.IsUnderProtectedPrefix(clean) {
		return fmt.Errorf("%w: %s is under a protected system path", ErrUnsafeDestination, path)
	}

	if resolved, err := sysfs.ResolveAndValidatePath(trim); err != nil {
		return fmt.Errorf("%w: %s: %v", ErrUnsafeDestination, path, err)
	} else if sysfs.IsUnderProtectedPrefix(resolved) {
		return fmt.Errorf("%w: %s resolves to protected path %s", ErrUnsafeDestination, path, resolved)
	}

	if fi, lerr := os.Lstat(trim); lerr == nil && fi.Mode()&os.ModeSymlink != 0 {
		if target, terr := filepath.EvalSymlinks(trim); terr == nil && sysfs.IsUnderProtectedPrefix(target) {
			return fmt.Errorf("%w: %s is a symlink to protected path %s", ErrUnsafeDestination, path, target)
		}
	}
	return nil
}

func canWipe(path string) error {
	trim := strings.TrimSpace(path)
	if trim == "" {
		return fmt.Errorf("%w: empty path", ErrUnsafeDestination)
	}
	if !filepath.IsAbs(trim) {
		return fmt.Errorf("%w: %s is not absolute", ErrUnsafeDestination, path)
	}
	clean := filepath.Clean(trim)
	if clean == "/" {
		return fmt.Errorf("%w: %s resolves to /", ErrUnsafeDestination, path)
	}

	if isManagedRoot(clean) {
		return nil
	}

	if sysfs.IsUnderProtectedPrefix(clean) {
		return fmt.Errorf("%w: %s is under a protected system path", ErrUnsafeDestination, path)
	}

	recordedDestsMu.RLock()
	_, recorded := recordedDests[clean]
	recordedDestsMu.RUnlock()
	if recorded {
		return nil
	}
	if resolved, err := sysfs.ResolveAndValidatePath(trim); err == nil {
		if sysfs.IsUnderProtectedPrefix(resolved) {
			return fmt.Errorf("%w: %s resolves to protected path %s", ErrUnsafeDestination, path, resolved)
		}
		recordedDestsMu.RLock()
		_, recorded = recordedDests[resolved]
		recordedDestsMu.RUnlock()
		if recorded {
			return nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: %s: %v", ErrUnsafeDestination, path, err)
	}

	return fmt.Errorf("%w: %s is not under a managed root and was not recorded", ErrUnsafeDestination, path)
}

// RecordDest registers a destination that a successful Fetch has just
// written to, so a later Wipe in the same process can clean it up even if
// the path lives outside the project-managed prefixes. Safe to call from
// multiple goroutines. Records both the cleaned and resolved forms so a
// later canWipe call matches either way.
func RecordDest(path string) {
	trim := strings.TrimSpace(path)
	if !filepath.IsAbs(trim) {
		return
	}
	clean := filepath.Clean(trim)
	recordedDestsMu.Lock()
	defer recordedDestsMu.Unlock()
	recordedDests[clean] = struct{}{}
	if resolved, err := sysfs.ResolveAndValidatePath(trim); err == nil {
		recordedDests[resolved] = struct{}{}
	}
}

func forgetDest(path string) {
	clean := filepath.Clean(strings.TrimSpace(path))
	recordedDestsMu.Lock()
	defer recordedDestsMu.Unlock()
	delete(recordedDests, clean)
	if resolved, err := sysfs.ResolveAndValidatePath(path); err == nil {
		delete(recordedDests, resolved)
	}
}
