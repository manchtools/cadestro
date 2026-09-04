package pkg

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
)

type dnf struct {
	r       sysexec.Runner
	command string
}

var _ Manager = (*dnf)(nil)

var nevraEpochRe = regexp.MustCompile(`-\d+:`)
var nevraVersionRe = regexp.MustCompile(`-\d`)

func parseNEVRAName(nevra string) string {
	if loc := nevraEpochRe.FindStringIndex(nevra); loc != nil {
		return nevra[:loc[0]]
	}
	loc := nevraVersionRe.FindStringIndex(nevra)
	if loc == nil {
		return nevra
	}
	return nevra[:loc[0]]
}

func splitRPMNameArch(value string) (string, string) {
	index := strings.LastIndexByte(value, '.')
	if index < 0 {
		return value, ""
	}
	return value[:index], value[index+1:]
}

func (d *dnf) Backend() Backend {
	if d.command == "dnf5" {
		return Dnf5
	}
	return Dnf
}

func (d *dnf) write(ctx context.Context, args ...string) (sysexec.Result, error) {
	res, err := runPriv(ctx, d.r, true, nil, d.command, args...)
	if err != nil {
		return sysexec.Result{}, err
	}
	return res, asCommandError(d.command, res)
}

// Version returns the dnf version string.
func (d *dnf) Version(ctx context.Context) (string, error) {
	out, err := readOut(ctx, d.r, d.command, "--version")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(strings.SplitN(out, "\n", 2)[0]), nil
}

func (d *dnf) Install(ctx context.Context, opts InstallOptions, specs ...InstallSpec) (sysexec.Result, error) {
	if len(specs) == 0 {
		return sysexec.Result{}, nil
	}
	for _, spec := range specs {
		if err := ValidatePackageName(spec.Name); err != nil {
			return sysexec.Result{}, err
		}
		if err := ValidatePackageVersion(spec.Version); err != nil {
			return sysexec.Result{}, err
		}
	}
	args := []string{"install", "-y"}
	if opts.AllowDowngrade && d.command == "dnf5" {
		args = append(args, "--allow-downgrade")
	}
	for _, spec := range specs {
		if spec.Version == "" {
			args = append(args, spec.Name)
		} else {
			args = append(args, fmt.Sprintf("%s-%s", spec.Name, spec.Version))
		}
	}
	return d.write(ctx, args...)
}

func (d *dnf) InstallLocal(ctx context.Context, path string, opts InstallLocalOptions) (sysexec.Result, error) {
	if err := ValidateLocalPackagePath(path); err != nil {
		return sysexec.Result{}, err
	}
	flags := []string{"install", "-y"}
	if opts.AllowUnsigned {
		flags = append(flags, "--nogpgcheck")
	}
	if opts.AllowDowngrade && d.command == "dnf5" {
		flags = append(flags, "--allow-downgrade")
	}
	return d.write(ctx, append(flags, path)...)
}

func (d *dnf) Remove(ctx context.Context, opts RemoveOptions, packages ...string) (sysexec.Result, error) {
	if len(packages) == 0 {
		return sysexec.Result{}, nil
	}
	if opts.Purge {
		return sysexec.Result{}, fmt.Errorf("dnf purge: %w", ErrUnsupported)
	}
	if err := ValidatePackageNames(packages); err != nil {
		return sysexec.Result{}, err
	}
	return d.write(ctx, append([]string{"remove", "-y"}, packages...)...)
}

// Update refreshes metadata via `dnf check-update` (exit 100 = updates available
// is a success, not an error).
func (d *dnf) Update(ctx context.Context) (sysexec.Result, error) {
	res, err := runPriv(ctx, d.r, true, nil, d.command, "check-update")
	if err != nil {
		return sysexec.Result{}, err
	}
	if res.ExitCode == 0 || res.ExitCode == 100 {
		return res, nil
	}
	return res, asCommandError(d.command, res)
}

// Upgrade upgrades the named packages, or all packages with no names.
func (d *dnf) Upgrade(ctx context.Context, packages ...string) (sysexec.Result, error) {
	if len(packages) == 0 {
		return sysexec.Result{}, nil
	}
	if err := ValidatePackageNames(packages); err != nil {
		return sysexec.Result{}, err
	}
	return d.write(ctx, append([]string{"upgrade", "-y"}, packages...)...)
}

// UpgradeAll performs a full system upgrade (dnf upgrade).
func (d *dnf) UpgradeAll(ctx context.Context) (sysexec.Result, error) {
	return d.write(ctx, "upgrade", "-y")
}

func (d *dnf) UpgradeSecurity(ctx context.Context) (sysexec.Result, error) {
	return d.write(ctx, "upgrade", "-y", "--security")
}

func (d *dnf) versionLockAvailable(ctx context.Context) error {
	_, ok, err := probe(ctx, d.r, d.command, "versionlock", "--help")
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%s versionlock: %w", d.command, ErrUnsupported)
	}
	return nil
}

func (d *dnf) Pin(ctx context.Context, packages ...string) (sysexec.Result, error) {
	if len(packages) == 0 {
		return sysexec.Result{}, nil
	}
	if err := ValidatePackageNames(packages); err != nil {
		return sysexec.Result{}, err
	}
	if err := d.versionLockAvailable(ctx); err != nil {
		return sysexec.Result{}, err
	}
	return d.write(ctx, append([]string{"versionlock", "add"}, packages...)...)
}

func (d *dnf) Unpin(ctx context.Context, packages ...string) (sysexec.Result, error) {
	if len(packages) == 0 {
		return sysexec.Result{}, nil
	}
	if err := ValidatePackageNames(packages); err != nil {
		return sysexec.Result{}, err
	}
	if err := d.versionLockAvailable(ctx); err != nil {
		return sysexec.Result{}, err
	}
	return d.write(ctx, append([]string{"versionlock", "delete"}, packages...)...)
}

// Autoremove removes packages installed only as now-unneeded dependencies.
func (d *dnf) Autoremove(ctx context.Context) (sysexec.Result, error) {
	return d.write(ctx, "autoremove", "-y")
}

// Search searches package names/summaries (exit 1 = no matches).
func (d *dnf) Search(ctx context.Context, query string) ([]SearchResult, error) {
	if err := ValidateSearchQuery(query); err != nil {
		return nil, err
	}
	res, err := runRead(ctx, d.r, d.command, "search", "-q", query)
	if err != nil {
		return nil, err
	}
	if res.ExitCode == 1 {
		return nil, nil
	}
	if res.ExitCode != 0 {
		return nil, asCommandError(d.command, res)
	}

	var results []SearchResult
	for line := range strings.SplitSeq(res.Stdout, "\n") {
		if strings.HasPrefix(line, "=") || line == "" {
			continue
		}
		parts := strings.SplitN(line, " : ", 2)
		if len(parts) < 2 {
			continue
		}
		name, _ := splitRPMNameArch(parts[0])
		results = append(results, SearchResult{
			Name:        name,
			Description: strings.TrimSpace(parts[1]),
		})
	}
	return results, nil
}

// List lists installed packages.
func (d *dnf) List(ctx context.Context) ([]Package, error) {
	out, err := readOut(ctx, d.r, "rpm", "-qa", "--queryformat",
		"%{NAME}\t%{VERSION}-%{RELEASE}\t%{ARCH}\t%{SIZE}\t%{SUMMARY}\n")
	if err != nil {
		return nil, err
	}

	var packages []Package
	for line := range strings.SplitSeq(out, "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) < 4 {
			continue
		}
		size, _ := strconv.ParseInt(fields[3], 10, 64)
		desc := ""
		if len(fields) > 4 {
			desc = fields[4]
		}
		packages = append(packages, Package{
			Name:         fields[0],
			Version:      fields[1],
			Architecture: fields[2],
			Status:       PackageStatusInstalled,
			Size:         size,
			Description:  desc,
		})
	}
	return packages, nil
}

// ListUpgradable lists packages with an available upgrade (check-update exit 100).
func (d *dnf) ListUpgradable(ctx context.Context) ([]PackageUpdate, error) {
	res, err := runRead(ctx, d.r, d.command, "check-update", "-q")
	if err != nil {
		return nil, err
	}
	if res.ExitCode != 0 && res.ExitCode != 100 {
		return nil, asCommandError(d.command, res)
	}

	var updates []PackageUpdate
	for line := range strings.SplitSeq(res.Stdout, "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		name, arch := splitRPMNameArch(fields[0])
		current, err := d.InstalledVersion(ctx, name)
		if err != nil {
			return nil, err
		}
		updates = append(updates, PackageUpdate{
			Name:           name,
			Architecture:   arch,
			NewVersion:     fields[1],
			Repository:     fields[2],
			CurrentVersion: current,
		})
	}
	return updates, nil
}

// Show returns detailed information about a package.
func (d *dnf) Show(ctx context.Context, name string) (*Package, error) {
	if err := ValidatePackageName(name); err != nil {
		return nil, err
	}
	out, err := readOut(ctx, d.r, d.command, "info", "-q", name)
	if err != nil {
		return nil, err
	}

	pkg := &Package{Name: name, Status: PackageStatusAvailable}
	for line := range strings.SplitSeq(out, "\n") {
		switch {
		case strings.HasPrefix(line, "Version"):
			pkg.Version = parseColonValue(line)
		case strings.HasPrefix(line, "Release"):
			if pkg.Version != "" {
				pkg.Version += "-" + parseColonValue(line)
			}
		case strings.HasPrefix(line, "Architecture"):
			pkg.Architecture = parseColonValue(line)
		case strings.HasPrefix(line, "Size"):

			if n, sizeOK := parseSize(parseColonValue(line)); sizeOK {
				pkg.Size = n
			}
		case strings.HasPrefix(line, "Summary"):
			pkg.Description = parseColonValue(line)
		case strings.HasPrefix(line, "Repository"):
			pkg.Repository = parseColonValue(line)
		}
	}

	installed, err := d.IsInstalled(ctx, name)
	if err != nil {
		return nil, err
	}
	if installed {
		pkg.Status = PackageStatusInstalled
	}
	return pkg, nil
}

// ListVersions lists the versions available for a package.
func (d *dnf) ListVersions(ctx context.Context, name string) (*VersionInfo, error) {
	if err := ValidatePackageName(name); err != nil {
		return nil, err
	}
	out, err := readOut(ctx, d.r, d.command, "list", "--showduplicates", "-q", name)
	if err != nil {
		return nil, err
	}

	info := &VersionInfo{Name: name}
	installed, err := d.InstalledVersion(ctx, name)
	if errors.Is(err, ErrNotFound) {
		err = nil
	}
	if err != nil {
		return nil, err
	}
	info.Installed = installed

	seen := make(map[string]bool)
	for line := range strings.SplitSeq(out, "\n") {
		if line == "" || strings.HasPrefix(line, "Installed") || strings.HasPrefix(line, "Available") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		version := fields[1]
		if seen[version] {
			continue
		}
		seen[version] = true
		info.Versions = append(info.Versions, AvailableVersion{
			Version:    version,
			Repository: fields[2],
		})
	}
	return info, nil
}

// LocalPackageInfo reads the canonical NAME/VERSION-RELEASE/ARCH out of a local
// .rpm via `rpm -qp --qf` (an unprivileged read). %{NAME} from a crafted .rpm is
// untrusted, so it is re-validated with ValidateRpmPackageName (the RPM grammar —
// which allows '+' for libstdc++ but no flag/metacharacter) before being
// returned. The shared rpmLocalPackageInfo helper keeps dnf and zypper in lockstep.
func (d *dnf) LocalPackageInfo(ctx context.Context, path string) (*LocalPackage, error) {
	return rpmLocalPackageInfo(ctx, d.r, path)
}

// IsInstalled reports whether a package is installed (rpm -q exits 0).
func (d *dnf) IsInstalled(ctx context.Context, name string) (bool, error) {
	if err := ValidatePackageName(name); err != nil {
		return false, err
	}
	res, err := runRead(ctx, d.r, "rpm", "-q", name)
	if err != nil {
		return false, err
	}
	return res.ExitCode == 0, nil
}

func (d *dnf) InstalledVersion(ctx context.Context, name string) (string, error) {
	if err := ValidatePackageName(name); err != nil {
		return "", err
	}
	res, err := runRead(ctx, d.r, "rpm", "-q", "--queryformat", "%{VERSION}-%{RELEASE}", name)
	if err != nil {
		return "", err
	}
	if res.ExitCode != 0 {
		return "", fmt.Errorf("%s package %s: %w", d.command, name, ErrNotFound)
	}
	return strings.TrimSpace(res.Stdout), nil
}

// InstalledCount returns the number of installed packages.
func (d *dnf) InstalledCount(ctx context.Context) (int, error) {
	out, err := readOut(ctx, d.r, "rpm", "-qa", "--qf", ".\n")
	if err != nil {
		return 0, err
	}
	return countNonEmptyLines(out), nil
}

// HasUpdates reports whether updates are available (dnf check-update exit 100).
func (d *dnf) HasUpdates(ctx context.Context) (bool, error) {
	res, err := runRead(ctx, d.r, d.command, "check-update", "-q")
	if err != nil {
		return false, err
	}
	switch res.ExitCode {
	case 0:
		return false, nil
	case 100:
		return true, nil
	default:
		return false, asCommandError(d.command, res)
	}
}

func (d *dnf) HasSecurityUpdates(ctx context.Context) (bool, error) {
	res, err := runRead(ctx, d.r, d.command, "check-update", "-q", "--security")
	if err != nil {
		return false, err
	}
	switch res.ExitCode {
	case 0:
		return false, nil
	case 100:
		return true, nil
	default:
		return false, asCommandError(d.command, res)
	}
}

func (d *dnf) IsPinned(ctx context.Context, name string) (bool, error) {
	if err := ValidatePackageName(name); err != nil {
		return false, err
	}
	if err := d.versionLockAvailable(ctx); err != nil {
		return false, err
	}
	out, err := readOut(ctx, d.r, d.command, "versionlock", "list", "-q")
	if err != nil {
		return false, err
	}
	for _, pinned := range versionLockNames(d.command, out) {
		if pinned == name {
			return true, nil
		}
	}
	return false, nil
}

func (d *dnf) ListPinned(ctx context.Context) ([]Package, error) {
	if err := d.versionLockAvailable(ctx); err != nil {
		return nil, err
	}
	out, err := readOut(ctx, d.r, d.command, "versionlock", "list", "-q")
	if err != nil {
		return nil, err
	}
	var packages []Package
	for _, name := range versionLockNames(d.command, out) {
		version, err := d.InstalledVersion(ctx, name)
		if err != nil {
			return nil, err
		}
		packages = append(packages, Package{Name: name, Version: version, Status: PackageStatusInstalled})
	}
	return packages, nil
}

func versionLockNames(command, out string) []string {
	var names []string
	for line := range strings.SplitSeq(out, "\n") {
		line = strings.TrimSpace(line)
		if command == "dnf5" {
			name, ok := strings.CutPrefix(line, "Package name:")
			if !ok {
				continue
			}
			name = strings.TrimSpace(name)
			if ValidatePackageName(name) == nil {
				names = append(names, name)
			}
			continue
		}
		if line != "" {
			names = append(names, parseNEVRAName(line))
		}
	}
	return names
}

func parseSize(s string) (int64, bool) {
	return parseSizeWithUnits(s, []sizeUnit{
		{" k", 1024},
		{" K", 1024},
		{" M", 1024 * 1024},
		{" G", 1024 * 1024 * 1024},
	})
}
