package pkg

// Package represents an installed or available package.
type Package struct {
	Name         string
	Version      string
	Architecture string
	Description  string
	Status       PackageStatus
	Size         int64
	Repository   string
}

// PackageStatus identifies whether a package is installed or available.
type PackageStatus string

const (
	// PackageStatusInstalled identifies an installed package.
	PackageStatusInstalled PackageStatus = "installed"
	// PackageStatusAvailable identifies a package available from a repository.
	PackageStatusAvailable PackageStatus = "available"
)

// LocalPackage is the identity read out of a LOCAL package file (a .deb, .rpm or
// pacman package on disk) by Manager.LocalPackageInfo. Name is the canonical
// package name — validated against the backend's package-name grammar before it
// is returned, because the value is read out of an attacker-influenced file.
// Version and Arch are best-effort extra fields (a backend that cannot report
// one leaves it empty).
type LocalPackage struct {
	Name    string
	Version string
	Arch    string
}

// PackageUpdate represents an available update for a package.
type PackageUpdate struct {
	Name           string
	CurrentVersion string
	NewVersion     string
	Architecture   string
	Repository     string
}

// SearchResult represents a package search result.
type SearchResult struct {
	Name        string
	Version     string
	Description string
	Repository  string
}

// VersionInfo represents available versions of a package.
type VersionInfo struct {
	Name      string
	Versions  []AvailableVersion
	Installed string
}

// AvailableVersion represents a specific version available for installation.
type AvailableVersion struct {
	Version    string
	Repository string
	Size       int64
}

// InstallSpec names one package and optionally pins its version.
type InstallSpec struct {
	Name    string
	Version string
}

// InstallOptions configures an Install call.
type InstallOptions struct {
	// AllowDowngrade permits installing a lower version than the one installed.
	AllowDowngrade bool
}

// InstallLocalOptions configures an InstallLocal call.
type InstallLocalOptions struct {
	// AllowDowngrade permits installing a package file whose version is lower
	// than the one currently installed. Honored on apt (--allow-downgrades),
	// dnf5 (--allow-downgrade) and zypper (--oldpackage). DNF4 exact package
	// specifications and local files already select the requested version.
	AllowDowngrade bool
	// AllowUnsigned skips the backend's GPG signature check for the local file.
	// It is secure-default-OFF: with the default (false) an unsigned local .rpm
	// is rejected by dnf/zypper. Set it ONLY when the file's authenticity is
	// established out of band — e.g. an HTTPS transport plus a verified SHA256
	// checksum — so the package manager's per-file GPG check is redundant. Per
	// backend:
	//   - dnf:    adds --nogpgcheck
	//   - zypper: adds --allow-unsigned-rpm (per-package; never the global
	//             --no-gpg-checks, which would also drop repo-metadata checks)
	//   - apt:    no-op — a local .deb carries no per-file signature to skip
	//   - pacman: NOT honored — `pacman -U` enforces the repo SigLevel and has no
	//             per-invocation bypass; the install stays signature-checked
	AllowUnsigned bool
}

// RemoveOptions configures a Remove call.
type RemoveOptions struct {
	// Purge also removes configuration/data where the backend distinguishes it.
	Purge bool
}
