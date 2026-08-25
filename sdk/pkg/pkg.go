// Package pkg provides a uniform package-manager abstraction for Linux.
//
// A Manager is built for an explicit Backend over an injected exec.Runner —
// the SDK keeps no global escalation state and every Manager is unit-testable
// with exectest.FakeRunner (no host, no sudo, no container):
//
//	r, _ := exec.NewRunner(exec.Sudo)
//	m, err := pkg.New(pkg.Apt, r)
//	if err != nil { ... }
//	if _, err := m.Install(ctx, pkg.InstallOptions{}, pkg.InstallSpec{Name: "vim"}, pkg.InstallSpec{Name: "git"}); err != nil { ... }
//
// Reads (Search/List/Show/IsInstalled/…) run unprivileged; mutations
// (Install/InstallLocal/Remove/Update/Upgrade/Pin/Unpin/Autoremove) run
// through the Runner's privilege backend. Every package-name, version, and
// local-file-path argument is validated before it can reach argv — there is no
// opt-out.
//
// Use Detect to discover which native backends are installed; it lists and
// never picks, so the caller decides.
package pkg

import (
	"context"
	"errors"
	"fmt"
	"os/exec"

	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
)

// ErrUnknownBackend is returned by New for a Backend the SDK does not implement
// (including the zero value). Fail-closed: no silent default.
var ErrUnknownBackend = errors.New("pkg: unknown package-manager backend")

var lookPath = exec.LookPath

// Backend identifies a supported package manager. The zero value is invalid
// (New rejects it); valid values start at 1.
type Backend int

const (
	// Apt is the Debian/Ubuntu package manager (apt / apt-get / dpkg).
	Apt Backend = iota + 1
	// Dnf is the Fedora/RHEL package manager (dnf / rpm).
	Dnf
	// Dnf5 is the Fedora/RHEL package manager backed by dnf5.
	Dnf5
	// Pacman is the Arch Linux package manager.
	Pacman
	// Zypper is the openSUSE/SLES package manager (zypper / rpm).
	Zypper
)

// String returns the canonical lowercase backend name.
func (b Backend) String() string {
	switch b {
	case Apt:
		return "apt"
	case Dnf:
		return "dnf"
	case Dnf5:
		return "dnf5"
	case Pacman:
		return "pacman"
	case Zypper:
		return "zypper"
	default:
		return fmt.Sprintf("Backend(%d)", int(b))
	}
}

// Manager is the uniform package-manager surface. Every method takes a context
// so the caller controls timeout/cancellation. Query methods return typed
// results; mutating methods return the command's output (an exec.Result carrying
// exit code, stdout and stderr) so callers can surface what the package manager
// actually did, plus an error — a non-zero exit becomes an *exec.CommandError
// carrying the exit code and stderr while the Result still carries the full
// stdout/stderr. The Result is populated on both the success and non-zero-exit
// paths; it is the zero Result only when the command could not be run at all
// (the error is then a plain runner error) or when the call is a validated
// no-op (e.g. an empty package list).
type Manager interface {
	// Backend reports which package-manager backend this Manager drives.
	Backend() Backend

	// Version returns the package-manager tool version string.
	Version(ctx context.Context) (string, error)
	// Search returns packages whose name/summary matches query.
	Search(ctx context.Context, query string) ([]SearchResult, error)
	// List returns the installed packages.
	List(ctx context.Context) ([]Package, error)
	// ListUpgradable returns packages with an available upgrade.
	ListUpgradable(ctx context.Context) ([]PackageUpdate, error)
	// Show returns detailed information about a single package.
	Show(ctx context.Context, name string) (*Package, error)
	// ListVersions returns the versions available for a package.
	ListVersions(ctx context.Context, name string) (*VersionInfo, error)
	// IsInstalled reports whether name is currently installed.
	IsInstalled(ctx context.Context, name string) (bool, error)
	// LocalPackageInfo reads a package's canonical name (and, where the backend
	// reports them, version and architecture) from a LOCAL package file already on
	// disk — a .deb, .rpm or pacman package — WITHOUT installing it. path must be
	// an absolute filesystem path (ValidateLocalPackagePath). The name a crafted
	// file embeds is untrusted, so it is validated against the backend's
	// package-name grammar before being returned; a flag-shaped or
	// metacharacter-bearing name is rejected.
	LocalPackageInfo(ctx context.Context, path string) (*LocalPackage, error)
	// InstalledVersion returns the installed version of name, or ErrNotFound.
	InstalledVersion(ctx context.Context, name string) (string, error)
	// InstalledCount returns the number of installed packages.
	InstalledCount(ctx context.Context) (int, error)
	// HasUpdates reports whether any update is available.
	HasUpdates(ctx context.Context) (bool, error)
	// HasSecurityUpdates reports whether security updates are available.
	HasSecurityUpdates(ctx context.Context) (bool, error)
	// IsPinned reports whether name is held back from upgrades.
	IsPinned(ctx context.Context, name string) (bool, error)
	// ListPinned returns the packages held back from upgrades.
	ListPinned(ctx context.Context) ([]Package, error)

	// Install installs each package specification in one transaction.
	Install(ctx context.Context, opts InstallOptions, specs ...InstallSpec) (sysexec.Result, error)
	// InstallLocal installs a package from a local file already on disk — a
	// downloaded .deb, .rpm, or pacman package — rather than by
	// name from a configured repository, resolving dependencies from the
	// configured repositories where the backend supports it (apt/dnf/zypper/
	// pacman). path must be an ABSOLUTE filesystem path to the package file;
	// fetching and verifying the artifact (https transport, checksum) is the
	// caller's responsibility. opts.AllowDowngrade permits a lower version than
	// the one currently installed; opts.AllowUnsigned skips the backend's GPG
	// check (dnf --nogpgcheck / zypper --allow-unsigned-rpm) for a file whose
	// authenticity is established out of band — secure-default-off.
	InstallLocal(ctx context.Context, path string, opts InstallLocalOptions) (sysexec.Result, error)
	// Remove removes the named packages. opts.Purge also deletes configuration
	// where the backend distinguishes it.
	Remove(ctx context.Context, opts RemoveOptions, packages ...string) (sysexec.Result, error)
	// Update refreshes the package metadata/database.
	Update(ctx context.Context) (sysexec.Result, error)
	// Upgrade upgrades the named packages. With NO names it is a no-op (not a
	// full upgrade) — an accidentally-empty list must never upgrade the whole
	// system. Use UpgradeAll for that.
	Upgrade(ctx context.Context, packages ...string) (sysexec.Result, error)
	// UpgradeAll performs a full system upgrade.
	UpgradeAll(ctx context.Context) (sysexec.Result, error)
	// UpgradeSecurity applies only security updates.
	UpgradeSecurity(ctx context.Context) (sysexec.Result, error)
	// Pin holds the named packages back from upgrades.
	Pin(ctx context.Context, packages ...string) (sysexec.Result, error)
	// Unpin releases the named packages so they upgrade again.
	Unpin(ctx context.Context, packages ...string) (sysexec.Result, error)
	// Autoremove removes packages installed only as now-unneeded dependencies.
	Autoremove(ctx context.Context) (sysexec.Result, error)
}

// New builds a Manager for backend b driven by runner. A nil runner or an
// unknown backend is rejected (fail-closed). New is pure — it does not probe
// the host; use Detect to learn which backends are installed.
func New(b Backend, runner sysexec.Runner) (Manager, error) {
	if runner == nil {
		return nil, fmt.Errorf("pkg: %w", sysexec.ErrRunnerRequired)
	}
	switch b {
	case Apt:
		return &apt{r: runner}, nil
	case Dnf:
		return &dnf{r: runner, command: "dnf"}, nil
	case Dnf5:
		return &dnf{r: runner, command: "dnf5"}, nil
	case Pacman:
		return &pacman{r: runner}, nil
	case Zypper:
		return &zypper{r: runner}, nil
	default:
		return nil, fmt.Errorf("%w: %d", ErrUnknownBackend, int(b))
	}
}

// Detect returns the package-manager backends whose primary binary resolves on
// PATH, in priority order. The result may be empty or hold several entries. It
// lists; it never picks — the caller chooses.
func Detect() []Backend {
	var found []Backend
	for _, c := range []struct {
		bin string
		b   Backend
	}{
		{"apt-get", Apt},
		{"dnf5", Dnf5},
		{"dnf", Dnf},
		{"pacman", Pacman},
		{"zypper", Zypper},
	} {
		if _, err := lookPath(c.bin); err == nil {
			if c.b == Dnf && len(found) > 0 && found[len(found)-1] == Dnf5 {
				continue
			}
			found = append(found, c.b)
		}
	}
	return found
}
