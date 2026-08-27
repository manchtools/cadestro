package fs

import (
	"path/filepath"
	"strings"
)

var dangerousPaths = map[string]bool{
	"/":       true,
	"/boot":   true,
	"/dev":    true,
	"/etc":    true,
	"/proc":   true,
	"/run":    true,
	"/sys":    true,
	"/usr":    true,
	"/var":    true,
	"/bin":    true,
	"/sbin":   true,
	"/lib":    true,
	"/lib64":  true,
	"/home":   true,
	"/root":   true,
	"/lib32":  true,
	"/libx32": true,
	"/media":  true,
	"/mnt":    true,
	"/opt":    true,
	"/srv":    true,
	"/tmp":    true,
	"/snap":   true,
}

// IsProtectedPath returns true if path is a system directory that should
// never be deleted. The path is cleaned and resolved to absolute before checking.
func IsProtectedPath(path string) bool {
	clean := filepath.Clean(path)
	if !filepath.IsAbs(clean) {
		abs, err := filepath.Abs(clean)
		if err != nil {
			return true
		}
		clean = abs
	}
	return dangerousPaths[clean]
}

var protectedPrefixRoots = []string{
	"/etc",
	"/boot",
	"/usr",
	"/home",
	"/root",
	"/var/lib",
	"/bin",
	"/sbin",
	"/lib",
	"/lib64",
	"/lib32",
	"/libx32",
	"/proc",
	"/sys",
	"/dev",
	"/run",
}

var protectedExactPaths = map[string]bool{
	"/":    true,
	"/var": true,
}

// IsUnderProtectedPrefix reports whether path is at or under a
// security-relevant system prefix that a managed directory delete must
// refuse. The path is cleaned and resolved to absolute before checking,
// so traversal tricks (/etc/../etc/sudoers.d) and relative inputs cannot
// dodge the guard. This is the deny-by-default predicate RemoveDir uses
// and that the agent's directory action reuses (WS6 #4, #12).
func IsUnderProtectedPrefix(path string) bool {
	clean := filepath.Clean(path)
	if !filepath.IsAbs(clean) {
		abs, err := filepath.Abs(clean)
		if err != nil {
			return true
		}
		clean = abs
	}
	if protectedExactPaths[clean] {
		return true
	}
	for _, root := range protectedPrefixRoots {
		if clean == root || strings.HasPrefix(clean, root+"/") {
			return true
		}
	}
	return false
}
