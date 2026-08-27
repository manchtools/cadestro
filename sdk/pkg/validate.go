package pkg

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var validPackageName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._+:/@~-]{0,255}$`)

// ValidatePackageName returns a non-nil error when name would be
// unsafe to pass as a positional argument to any of the supported
// package managers. Callers MUST invoke this (or
// ValidatePackageNames) at the top of every public method that
// accepts a package name, before any exec.CommandContext call.
func ValidatePackageName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: package name is empty", ErrInvalidArgument)
	}
	if !validPackageName.MatchString(name) {
		return fmt.Errorf("%w: invalid package name %q: must start with [a-zA-Z0-9] and contain only [a-zA-Z0-9._+:/@~-]", ErrInvalidArgument, name)
	}
	return nil
}

// ValidatePackageNames runs ValidatePackageName against every entry.
// Returns the first rejection; does not try to be exhaustive —
// actions are signed and a rejection here is a caller bug, not an
// adversarial probe to enumerate.
func ValidatePackageNames(names []string) error {
	for _, n := range names {
		if err := ValidatePackageName(n); err != nil {
			return err
		}
	}
	return nil
}

var validPackageVersion = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._+:~^-]{0,127}$`)

// ValidatePackageVersion checks a version string before it lands in
// `<name>=<version>` (apt) or similar argv constructs. Empty version
// is treated as "no version pinned" and accepted; any non-empty
// version must match the cross-distro grammar in validPackageVersion.
func ValidatePackageVersion(version string) error {
	if version == "" {
		return nil
	}
	if !validPackageVersion.MatchString(version) {
		return fmt.Errorf("%w: invalid package version %q: must start with [a-zA-Z0-9] and contain only [a-zA-Z0-9._+:~^-]", ErrInvalidArgument, version)
	}
	return nil
}

var validRpmPackageName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._+-]{0,255}$`)

// ValidateRpmPackageName returns a non-nil error when name is not a
// safe RPM %{NAME} to pass to `rpm -q`/`rpm -e`. The name a crafted
// .rpm reports is untrusted; callers MUST validate it before it reaches
// argv. Mirrors ValidatePackageName / the deb-side validDebPkgName.
func ValidateRpmPackageName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: rpm package name is empty", ErrInvalidArgument)
	}
	if !validRpmPackageName.MatchString(name) {
		return fmt.Errorf("%w: invalid rpm package name %q: must start with [a-zA-Z0-9] and contain only [a-zA-Z0-9._+-]", ErrInvalidArgument, name)
	}
	return nil
}

// ValidateLocalPackagePath validates a filesystem path before it reaches a
// package manager as the operand of a local-file install (apt-get install
// <file>, dnf install <file>, pacman -U <file>, zypper install <file>, flatpak
// install <bundle>). It is NOT a package name, so the package-name grammar does
// not apply; what matters is that the path cannot masquerade as an option and
// cannot smuggle a config/log-injecting control character:
//
//   - absolute (a leading '/'): a relative path is ambiguous w.r.t. the
//     privileged process's working directory, and an absolute path can never be
//     flag-shaped. Callers pass a concrete file location (typically a temp file
//     they downloaded and checksum-verified), so this costs them nothing.
//   - no ".." segment: defence-in-depth against a traversing path, mirroring
//     ValidateGpgKeyRef.
//   - no control characters (incl. NUL, newline, tab) or DEL: a space is left
//     alone (it is argv-safe and legal in a path), but control characters are
//     refused so a crafted path can neither break a log line nor confuse a tool.
//
// Existence is NOT checked: the Manager has no host access (it drives an
// injected Runner), and a missing file surfaces cleanly as the tool's own
// non-zero exit. The absolute-path requirement — not a "--" end-of-options
// separator — is what keeps the path from being read as an option, because some
// tools (dnf5) reject "--" for their install/search subcommands entirely.
func ValidateLocalPackagePath(path string) error {
	if path == "" {
		return fmt.Errorf("%w: local package path is empty", ErrInvalidArgument)
	}
	if hasControlChar(path) {
		return fmt.Errorf("%w: local package path contains control characters", ErrInvalidArgument)
	}
	if !strings.HasPrefix(path, "/") {
		return fmt.Errorf("%w: local package path %q must be an absolute path", ErrInvalidArgument, path)
	}
	if strings.Contains(path, "..") {
		return fmt.Errorf("%w: local package path %q must not contain '..'", ErrInvalidArgument, path)
	}
	return nil
}

// ValidateSearchQuery guards the free-text query passed to a package-manager
// search. A search term is not a fixed grammar (it may carry '+', '.', glob
// characters, etc.), so it is deliberately NOT run through validPackageName;
// the one thing that must be refused is a value that could be reparsed as an
// OPTION. A leading '-' is rejected — a search term never legitimately starts
// with a dash, and a "--" end-of-options separator is NOT portable here (dnf5's
// `search` subcommand rejects "--"), so validation is the honest defense. Control
// characters are rejected too. An empty query is allowed (a backend may list
// everything).
func ValidateSearchQuery(query string) error {
	if strings.HasPrefix(query, "-") {
		return fmt.Errorf("%w: invalid search query %q: must not start with '-'", ErrInvalidArgument, query)
	}
	if hasControlChar(query) {
		return fmt.Errorf("%w: search query contains control characters", ErrInvalidArgument)
	}
	return nil
}

func hasControlChar(s string) bool {
	for _, r := range s {
		if r < ' ' || r == 0x7f {
			return true
		}
	}
	return false
}

var validRemoteName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$`)

// ValidateRemoteName returns a non-nil error when name is not a safe
// flatpak remote alias to pass as an operand to `flatpak install`. A
// flag-shaped remote (`--from=…`) would otherwise be parsed as an
// option.
func ValidateRemoteName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: flatpak remote name is empty", ErrInvalidArgument)
	}
	if !validRemoteName.MatchString(name) {
		return fmt.Errorf("%w: invalid flatpak remote name %q: must start with [a-zA-Z0-9] and contain only [a-zA-Z0-9._-]", ErrInvalidArgument, name)
	}
	return nil
}

func hasCtrlOrSpace(s string) bool {
	for _, r := range s {
		if r <= ' ' || r == 0x7f {
			return true
		}
	}
	return false
}

// ValidateRepoBaseURL validates a dnf baseurl / zypper url / pacman
// server. A repository base URL is where the package manager fetches
// ROOT-installed packages, so it must be https (no http/ftp/file — the
// transport is the only thing standing between a MITM and arbitrary
// root code), a real URL with a host, and free of control characters.
// Package-manager template variables ($releasever, $arch, $basearch)
// survive url.Parse and are intentionally allowed.
//
// NOTE: apt is deliberately NOT validated through here — apt's security
// model is the gpg-signed Release file, so an http transport with a
// trusted key is a legitimate, long-standing configuration.
func ValidateRepoBaseURL(rawURL string) error {
	if rawURL == "" {
		return fmt.Errorf("%w: repository base URL is empty", ErrInvalidArgument)
	}
	if hasCtrlOrSpace(rawURL) {
		return fmt.Errorf("%w: repository base URL contains whitespace or control characters", ErrInvalidArgument)
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("%w: repository base URL is not a valid URL: %w", ErrInvalidArgument, err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("%w: repository base URL must use https, got scheme %q", ErrInvalidArgument, u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("%w: repository base URL has no host", ErrInvalidArgument)
	}
	return nil
}

// ValidateGpgKeyRef validates a dnf/zypper Gpgkey reference before it
// reaches `rpm --import`. Accepted iff it is (a) an https URL with a
// host, (b) a file:// absolute path (file:///… — empty host, no `..`),
// or (c) a bare absolute filesystem path (no `..`). Everything else is
// refused: a leading '-' (option injection into `rpm --import`),
// plaintext http (MITM of the trust anchor itself), rpm's `ext::`
// external transport (which executes a command), and relative or
// traversing paths.
func ValidateGpgKeyRef(ref string) error {
	if ref == "" {
		return fmt.Errorf("%w: gpg key ref is empty", ErrInvalidArgument)
	}
	if hasCtrlOrSpace(ref) {
		return fmt.Errorf("%w: gpg key ref contains whitespace or control characters", ErrInvalidArgument)
	}
	if strings.HasPrefix(ref, "-") {
		return fmt.Errorf("%w: gpg key ref %q is flag-shaped (leading '-')", ErrInvalidArgument, ref)
	}
	switch {
	case strings.HasPrefix(ref, "https://"):
		u, err := url.Parse(ref)
		if err != nil {
			return fmt.Errorf("%w: gpg key ref is not a valid URL: %w", ErrInvalidArgument, err)
		}
		if u.Host == "" {
			return fmt.Errorf("%w: gpg key https ref has no host", ErrInvalidArgument)
		}
		return nil
	case strings.HasPrefix(ref, "file://"):
		u, err := url.Parse(ref)
		if err != nil {
			return fmt.Errorf("%w: gpg key ref is not a valid URL: %w", ErrInvalidArgument, err)
		}
		if u.Host != "" {
			return fmt.Errorf("%w: gpg key file ref must be file:///absolute/path (no host)", ErrInvalidArgument)
		}
		if !strings.HasPrefix(u.Path, "/") || strings.Contains(u.Path, "..") {
			return fmt.Errorf("%w: gpg key file ref must be an absolute path with no '..'", ErrInvalidArgument)
		}
		return nil
	case strings.HasPrefix(ref, "/"):
		if strings.Contains(ref, "..") {
			return fmt.Errorf("%w: gpg key path ref must not contain '..'", ErrInvalidArgument)
		}
		return nil
	default:
		return fmt.Errorf("%w: gpg key ref %q must be an https URL, a file:// absolute path, or an absolute filesystem path", ErrInvalidArgument, ref)
	}
}
