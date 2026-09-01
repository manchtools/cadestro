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

type zypper struct {
	r sysexec.Runner
}

var _ Manager = (*zypper)(nil)

var zypperLockRe = regexp.MustCompile(`^\s*\d+\s*\|\s*(\S+)`)

func isZypperInfoExit(code int) bool {
	return code == 0 || (code >= 100 && code <= 103)
}

func (z *zypper) Backend() Backend { return Zypper }

func (z *zypper) write(ctx context.Context, args ...string) (sysexec.Result, error) {
	res, err := runPriv(ctx, z.r, true, nil, "zypper", args...)
	if err != nil {
		return sysexec.Result{}, err
	}
	if isZypperInfoExit(res.ExitCode) {
		return res, nil
	}
	return res, asCommandError("zypper", res)
}

// Version returns the zypper version string.
func (z *zypper) Version(ctx context.Context) (string, error) {
	out, err := readOut(ctx, z.r, "zypper", "--version")
	if err != nil {
		return "", err
	}
	parts := strings.Fields(out)
	if len(parts) >= 2 {
		return parts[1], nil
	}
	return "", nil
}

func (z *zypper) Install(ctx context.Context, opts InstallOptions, specs ...InstallSpec) (sysexec.Result, error) {
	if len(specs) == 0 {
		return sysexec.Result{}, nil
	}
	args := []string{"--non-interactive", "install"}
	if opts.AllowDowngrade {
		args = append(args, "--oldpackage")
	}
	for _, spec := range specs {
		if err := ValidatePackageName(spec.Name); err != nil {
			return sysexec.Result{}, err
		}
		if err := ValidatePackageVersion(spec.Version); err != nil {
			return sysexec.Result{}, err
		}
		if spec.Version == "" {
			args = append(args, spec.Name)
		} else {
			args = append(args, fmt.Sprintf("%s=%s", spec.Name, spec.Version))
		}
	}
	return z.write(ctx, args...)
}

// InstallLocal installs a local .rpm file through zypper, resolving its
// dependencies from the configured repositories. opts.AllowDowngrade adds
// --oldpackage so a file older than the installed version is accepted.
// opts.AllowUnsigned adds the per-package --allow-unsigned-rpm (NOT the global
// --no-gpg-checks, which would also drop repository-metadata verification).
// ValidateLocalPackagePath requires an absolute path, so the operand can never
// be flag-shaped.
func (z *zypper) InstallLocal(ctx context.Context, path string, opts InstallLocalOptions) (sysexec.Result, error) {
	if err := ValidateLocalPackagePath(path); err != nil {
		return sysexec.Result{}, err
	}
	flags := []string{"--non-interactive", "install"}
	if opts.AllowUnsigned {
		flags = append(flags, "--allow-unsigned-rpm")
	}
	if opts.AllowDowngrade {
		flags = append(flags, "--oldpackage")
	}
	return z.write(ctx, append(flags, path)...)
}

func (z *zypper) Remove(ctx context.Context, opts RemoveOptions, packages ...string) (sysexec.Result, error) {
	if len(packages) == 0 {
		return sysexec.Result{}, nil
	}
	if opts.Purge {
		return sysexec.Result{}, fmt.Errorf("zypper purge: %w", ErrUnsupported)
	}
	if err := ValidatePackageNames(packages); err != nil {
		return sysexec.Result{}, err
	}
	return z.write(ctx, append([]string{"--non-interactive", "remove"}, packages...)...)
}

// Update refreshes the repositories.
func (z *zypper) Update(ctx context.Context) (sysexec.Result, error) {
	return z.write(ctx, "--non-interactive", "refresh")
}

func (z *zypper) Upgrade(ctx context.Context, packages ...string) (sysexec.Result, error) {
	if len(packages) == 0 {
		return sysexec.Result{}, nil
	}
	if err := ValidatePackageNames(packages); err != nil {
		return sysexec.Result{}, err
	}
	return z.write(ctx, append([]string{"--non-interactive", "update"}, packages...)...)
}

func (z *zypper) UpgradeAll(ctx context.Context) (sysexec.Result, error) {
	return z.write(ctx, "--non-interactive", "update")
}

func (z *zypper) UpgradeSecurity(ctx context.Context) (sysexec.Result, error) {
	return z.write(ctx, "--non-interactive", "patch", "--category", "security")
}

func (z *zypper) Autoremove(context.Context) (sysexec.Result, error) {
	return sysexec.Result{}, fmt.Errorf("zypper autoremove: %w", ErrUnsupported)
}

// Search searches packages (exit 104 = no matches).
func (z *zypper) Search(ctx context.Context, query string) ([]SearchResult, error) {
	if err := ValidateSearchQuery(query); err != nil {
		return nil, err
	}
	res, err := runRead(ctx, z.r, "zypper", "--non-interactive", "search", query)
	if err != nil {
		return nil, err
	}
	if res.ExitCode == 104 {
		return nil, nil
	}
	if res.ExitCode != 0 {
		return nil, asCommandError("zypper", res)
	}

	var results []SearchResult
	headerPassed := false
	for line := range strings.SplitSeq(res.Stdout, "\n") {
		if strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+-") {
			headerPassed = true
			continue
		}
		if !headerPassed {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) < 3 {
			continue
		}
		name := strings.TrimSpace(parts[1])
		if name == "" {
			continue
		}
		results = append(results, SearchResult{
			Name:        name,
			Description: strings.TrimSpace(parts[2]),
		})
	}
	return results, nil
}

func (z *zypper) HasSecurityUpdates(ctx context.Context) (bool, error) {
	res, err := runRead(ctx, z.r, "zypper", "--non-interactive", "list-patches", "--category", "security")
	if err != nil {
		return false, err
	}
	if res.ExitCode == 100 || res.ExitCode == 101 {
		return true, nil
	}
	if !isZypperInfoExit(res.ExitCode) {
		return false, asCommandError("zypper", res)
	}
	for _, line := range strings.Split(res.Stdout, "\n") {
		if strings.Contains(line, "v |") || strings.Contains(line, "i |") {
			return true, nil
		}
	}
	return false, nil
}

// List lists installed packages (via rpm).
func (z *zypper) List(ctx context.Context) ([]Package, error) {
	out, err := readOut(ctx, z.r, "rpm", "-qa", "--queryformat",
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

// ListUpgradable lists packages with an available upgrade. zypper signals
// "updates available" / "patches needed" with informational exit codes
// (100–103) that are not failures, so those are accepted alongside 0.
func (z *zypper) ListUpgradable(ctx context.Context) ([]PackageUpdate, error) {
	res, err := runRead(ctx, z.r, "zypper", "--non-interactive", "list-updates")
	if err != nil {
		return nil, err
	}
	if !isZypperInfoExit(res.ExitCode) {
		return nil, asCommandError("zypper", res)
	}

	var updates []PackageUpdate
	headerPassed := false
	for line := range strings.SplitSeq(res.Stdout, "\n") {
		if strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+-") {
			headerPassed = true
			continue
		}
		if !headerPassed {
			continue
		}

		parts := strings.Split(line, "|")
		if len(parts) < 5 {
			continue
		}
		name := strings.TrimSpace(parts[2])
		if name == "" {
			continue
		}
		arch := ""
		if len(parts) > 5 {
			arch = strings.TrimSpace(parts[5])
		}
		updates = append(updates, PackageUpdate{
			Name:           name,
			Repository:     strings.TrimSpace(parts[1]),
			CurrentVersion: strings.TrimSpace(parts[3]),
			NewVersion:     strings.TrimSpace(parts[4]),
			Architecture:   arch,
		})
	}
	return updates, nil
}

// Show returns detailed information about a package.
func (z *zypper) Show(ctx context.Context, name string) (*Package, error) {
	if err := ValidatePackageName(name); err != nil {
		return nil, err
	}
	out, err := readOut(ctx, z.r, "zypper", "--non-interactive", "info", name)
	if err != nil {
		return nil, err
	}

	pkg := &Package{Name: name, Status: PackageStatusAvailable}
	for line := range strings.SplitSeq(out, "\n") {
		switch {
		case strings.HasPrefix(line, "Version"):
			pkg.Version = parseColonValue(line)
		case strings.HasPrefix(line, "Arch"):
			pkg.Architecture = parseColonValue(line)
		case strings.HasPrefix(line, "Summary"):
			pkg.Description = parseColonValue(line)
		case strings.HasPrefix(line, "Installed Size"):

			if n, sizeOK := parseZypperSize(parseColonValue(line)); sizeOK {
				pkg.Size = n
			}
		case strings.HasPrefix(line, "Repository"):
			pkg.Repository = parseColonValue(line)
		case strings.HasPrefix(line, "Status"):
			if strings.Contains(strings.ToLower(parseColonValue(line)), "installed") {
				pkg.Status = PackageStatusInstalled
			}
		}
	}

	installed, err := z.IsInstalled(ctx, name)
	if err != nil {
		return nil, err
	}
	if installed {
		pkg.Status = PackageStatusInstalled
	}
	return pkg, nil
}

// ListVersions lists the versions available for a package.
func (z *zypper) ListVersions(ctx context.Context, name string) (*VersionInfo, error) {
	if err := ValidatePackageName(name); err != nil {
		return nil, err
	}

	res, err := runRead(ctx, z.r, "zypper", "--non-interactive", "search", "-s", "--match-exact", name)
	if err != nil {
		return nil, err
	}

	info := &VersionInfo{Name: name}
	installed, err := z.InstalledVersion(ctx, name)
	if errors.Is(err, ErrNotFound) {
		err = nil
	}
	if err != nil {
		return nil, err
	}
	info.Installed = installed

	if res.ExitCode == 104 {
		return info, nil
	}
	if res.ExitCode != 0 {
		return nil, asCommandError("zypper", res)
	}

	seen := make(map[string]bool)
	headerPassed := false
	for line := range strings.SplitSeq(res.Stdout, "\n") {
		if strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+-") {
			headerPassed = true
			continue
		}
		if !headerPassed {
			continue
		}

		parts := strings.Split(line, "|")
		if len(parts) < 6 {
			continue
		}
		if strings.TrimSpace(parts[1]) != name {
			continue
		}
		version := strings.TrimSpace(parts[3])
		if seen[version] {
			continue
		}
		seen[version] = true
		info.Versions = append(info.Versions, AvailableVersion{
			Version:    version,
			Repository: strings.TrimSpace(parts[5]),
		})
	}
	return info, nil
}

// LocalPackageInfo reads the canonical NAME/VERSION-RELEASE/ARCH out of a local
// .rpm via the shared rpmLocalPackageInfo helper (an unprivileged `rpm -qp --qf`
// read), re-validating the untrusted %{NAME} with ValidateRpmPackageName before
// returning it.
func (z *zypper) LocalPackageInfo(ctx context.Context, path string) (*LocalPackage, error) {
	return rpmLocalPackageInfo(ctx, z.r, path)
}

// IsInstalled reports whether a package is installed (rpm -q exits 0).
func (z *zypper) IsInstalled(ctx context.Context, name string) (bool, error) {
	if err := ValidatePackageName(name); err != nil {
		return false, err
	}
	res, err := runRead(ctx, z.r, "rpm", "-q", name)
	if err != nil {
		return false, err
	}
	return res.ExitCode == 0, nil
}

func (z *zypper) InstalledVersion(ctx context.Context, name string) (string, error) {
	if err := ValidatePackageName(name); err != nil {
		return "", err
	}
	res, err := runRead(ctx, z.r, "rpm", "-q", "--queryformat", "%{VERSION}-%{RELEASE}", name)
	if err != nil {
		return "", err
	}
	if res.ExitCode != 0 {
		return "", fmt.Errorf("zypper package %s: %w", name, ErrNotFound)
	}
	return strings.TrimSpace(res.Stdout), nil
}

// InstalledCount returns the number of installed packages (via rpm).
func (z *zypper) InstalledCount(ctx context.Context) (int, error) {
	out, err := readOut(ctx, z.r, "rpm", "-qa", "--qf", ".\n")
	if err != nil {
		return 0, err
	}
	return countNonEmptyLines(out), nil
}

// HasUpdates reports whether any update is available (list-updates: exit 100, or
// exit 0 with an update/patch table row).
func (z *zypper) HasUpdates(ctx context.Context) (bool, error) {
	res, err := runRead(ctx, z.r, "zypper", "--non-interactive", "list-updates")
	if err != nil {
		return false, err
	}
	if res.ExitCode == 100 || res.ExitCode == 101 {
		return true, nil
	}
	if !isZypperInfoExit(res.ExitCode) {

		return false, asCommandError("zypper", res)
	}
	for line := range strings.SplitSeq(res.Stdout, "\n") {
		if strings.Contains(line, "v |") || strings.Contains(line, "i |") {
			return true, nil
		}
	}
	return false, nil
}

// Pin holds packages back (zypper addlock).
func (z *zypper) Pin(ctx context.Context, packages ...string) (sysexec.Result, error) {
	if len(packages) == 0 {
		return sysexec.Result{}, nil
	}
	if err := ValidatePackageNames(packages); err != nil {
		return sysexec.Result{}, err
	}
	return z.write(ctx, append([]string{"--non-interactive", "addlock"}, packages...)...)
}

// Unpin releases held packages (zypper removelock).
func (z *zypper) Unpin(ctx context.Context, packages ...string) (sysexec.Result, error) {
	if len(packages) == 0 {
		return sysexec.Result{}, nil
	}
	if err := ValidatePackageNames(packages); err != nil {
		return sysexec.Result{}, err
	}
	return z.write(ctx, append([]string{"--non-interactive", "removelock"}, packages...)...)
}

// ListPinned lists locked packages.
func (z *zypper) ListPinned(ctx context.Context) ([]Package, error) {
	out, err := readOut(ctx, z.r, "zypper", "--non-interactive", "locks")
	if err != nil {
		return nil, err
	}

	var packages []Package
	headerPassed := false
	for line := range strings.SplitSeq(out, "\n") {
		if strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+-") {
			headerPassed = true
			continue
		}
		if !headerPassed {
			continue
		}
		m := zypperLockRe.FindStringSubmatch(line)
		if len(m) < 2 {
			continue
		}
		version, err := z.InstalledVersion(ctx, m[1])
		if err != nil {
			return nil, err
		}
		packages = append(packages, Package{
			Name:    m[1],
			Version: version,
			Status:  PackageStatusInstalled,
		})
	}
	return packages, nil
}

// IsPinned reports whether a package is locked.
func (z *zypper) IsPinned(ctx context.Context, name string) (bool, error) {
	if err := ValidatePackageName(name); err != nil {
		return false, err
	}
	out, ok, err := probe(ctx, z.r, "zypper", "--non-interactive", "locks")
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	re := regexp.MustCompile(`^\s*\d+\s*\|\s*` + regexp.QuoteMeta(name) + `\s*\|`)
	for line := range strings.SplitSeq(out, "\n") {
		if re.MatchString(line) {
			return true, nil
		}
	}
	return false, nil
}

func parseZypperSize(s string) (int64, bool) {
	return parseSizeWithUnits(s, []sizeUnit{
		{" KiB", 1024},
		{" MiB", 1024 * 1024},
		{" GiB", 1024 * 1024 * 1024},
		{" B", 1},
	})
}
