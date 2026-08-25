package pkg

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
)

type apt struct {
	r sysexec.Runner
}

var _ Manager = (*apt)(nil)

var aptWriteEnv = []string{"DEBIAN_FRONTEND=noninteractive"}

var dpkgConfOptions = []string{
	"-o", "Dpkg::Options::=--force-confdef",
	"-o", "Dpkg::Options::=--force-confold",
}

func (a *apt) Backend() Backend { return Apt }

func (a *apt) write(ctx context.Context, cmd string, args ...string) (sysexec.Result, error) {
	if cmd == "apt" || cmd == "apt-get" {
		cmd = "apt-get"
	}
	res, err := runPriv(ctx, a.r, true, aptWriteEnv, cmd, args...)
	if err != nil {
		return sysexec.Result{}, err
	}
	return res, asCommandError(cmd, res)
}

// Version returns the apt version string.
func (a *apt) Version(ctx context.Context) (string, error) {
	out, err := readOut(ctx, a.r, "apt-get", "--version")
	if err != nil {
		return "", err
	}
	parts := strings.Fields(out)
	if len(parts) >= 2 {
		return parts[1], nil
	}
	return "", nil
}

func (a *apt) Install(ctx context.Context, opts InstallOptions, specs ...InstallSpec) (sysexec.Result, error) {
	if len(specs) == 0 {
		return sysexec.Result{}, nil
	}
	args := []string{"install", "-y"}
	if opts.AllowDowngrade {
		args = append(args, "--allow-downgrades")
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
	return a.write(ctx, "apt", args...)
}

// InstallLocal installs a local .deb file through apt-get install, which —
// unlike a bare `dpkg -i` — resolves the package's dependencies from the
// configured repositories. ValidateLocalPackagePath requires an absolute path,
// so the operand can never be flag-shaped; the conffile-default options keep a
// postinst that touches a conffile non-interactive. opts.AllowUnsigned is a
// no-op here — a local .deb carries no per-file signature to skip.
func (a *apt) InstallLocal(ctx context.Context, path string, opts InstallLocalOptions) (sysexec.Result, error) {
	if err := ValidateLocalPackagePath(path); err != nil {
		return sysexec.Result{}, err
	}
	flags := []string{"install", "-y"}
	flags = append(flags, dpkgConfOptions...)
	if opts.AllowDowngrade {
		flags = append(flags, "--allow-downgrades")
	}
	return a.write(ctx, "apt", append(flags, path)...)
}

// Remove removes packages; opts.Purge deletes configuration files too.
func (a *apt) Remove(ctx context.Context, opts RemoveOptions, packages ...string) (sysexec.Result, error) {
	if len(packages) == 0 {
		return sysexec.Result{}, nil
	}
	if err := ValidatePackageNames(packages); err != nil {
		return sysexec.Result{}, err
	}
	verb := "remove"
	if opts.Purge {
		verb = "purge"
	}
	args := append([]string{verb, "-y"}, packages...)
	return a.write(ctx, "apt", args...)
}

// Update refreshes the package index.
func (a *apt) Update(ctx context.Context) (sysexec.Result, error) {
	return a.write(ctx, "apt", "update")
}

func (a *apt) Upgrade(ctx context.Context, packages ...string) (sysexec.Result, error) {
	if len(packages) == 0 {
		return sysexec.Result{}, nil
	}
	if err := ValidatePackageNames(packages); err != nil {
		return sysexec.Result{}, err
	}
	args := append([]string{"install", "-y", "--only-upgrade"}, dpkgConfOptions...)
	args = append(args, packages...)
	return a.write(ctx, "apt", args...)
}

func (a *apt) UpgradeAll(ctx context.Context) (sysexec.Result, error) {
	args := append([]string{"upgrade", "-y"}, dpkgConfOptions...)
	return a.write(ctx, "apt-get", args...)
}

func (a *apt) UpgradeSecurity(ctx context.Context) (sysexec.Result, error) {
	return a.securityUpgrade(ctx)
}

func (a *apt) securityUpgrade(ctx context.Context) (sysexec.Result, error) {
	bin, err := resolveUnattendedUpgrade()
	if err != nil {
		return sysexec.Result{}, err
	}
	return a.write(ctx, bin, "-v")
}

var unattendedUpgradeBinPaths = []string{
	"/usr/bin/unattended-upgrade",
	"/usr/sbin/unattended-upgrade",
}

func resolveUnattendedUpgrade() (string, error) {
	for _, p := range unattendedUpgradeBinPaths {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p, nil
		}
	}
	if p, err := lookPath("unattended-upgrade"); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("%w: unattended-upgrade not found — install the unattended-upgrades package for apt security-only upgrades", sysexec.ErrBackendUnavailable)
}

// Pin holds packages (apt-mark hold).
func (a *apt) Pin(ctx context.Context, packages ...string) (sysexec.Result, error) {
	if len(packages) == 0 {
		return sysexec.Result{}, nil
	}
	if err := ValidatePackageNames(packages); err != nil {
		return sysexec.Result{}, err
	}
	return a.write(ctx, "apt-mark", append([]string{"hold"}, packages...)...)
}

// Unpin releases held packages (apt-mark unhold).
func (a *apt) Unpin(ctx context.Context, packages ...string) (sysexec.Result, error) {
	if len(packages) == 0 {
		return sysexec.Result{}, nil
	}
	if err := ValidatePackageNames(packages); err != nil {
		return sysexec.Result{}, err
	}
	return a.write(ctx, "apt-mark", append([]string{"unhold"}, packages...)...)
}

// Autoremove removes packages installed only as now-unneeded dependencies.
func (a *apt) Autoremove(ctx context.Context) (sysexec.Result, error) {
	return a.write(ctx, "apt", "autoremove", "-y")
}

// Search searches package names. It always uses `apt-cache search`, which emits
// the stable single-line "name - description" format the parser expects; `apt
// search` produces a multi-line, presentation-oriented format that would not
// parse, and `--names-only` would drop the description (and the separator).
func (a *apt) Search(ctx context.Context, query string) ([]SearchResult, error) {
	if err := ValidateSearchQuery(query); err != nil {
		return nil, err
	}
	out, err := readOut(ctx, a.r, "apt-cache", "search", query)
	if err != nil {
		return nil, err
	}

	var results []SearchResult
	for line := range strings.SplitSeq(out, "\n") {
		parts := strings.SplitN(line, " - ", 2)
		if len(parts) < 2 {
			continue
		}
		results = append(results, SearchResult{
			Name:        strings.TrimSpace(parts[0]),
			Description: strings.TrimSpace(parts[1]),
		})
	}
	return results, nil
}

// List lists installed packages.
func (a *apt) List(ctx context.Context) ([]Package, error) {
	out, err := readOut(ctx, a.r, "dpkg-query", "-W",
		"-f=${Package}\t${Version}\t${Architecture}\t${Status}\t${Installed-Size}\t${Description}\n")
	if err != nil {
		return nil, err
	}

	var packages []Package
	for line := range strings.SplitSeq(out, "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) < 5 {
			continue
		}
		if !strings.Contains(fields[3], "installed") {
			continue
		}
		size, _ := strconv.ParseInt(fields[4], 10, 64)
		desc := ""
		if len(fields) > 5 {
			desc = fields[5]
		}
		packages = append(packages, Package{
			Name:         fields[0],
			Version:      fields[1],
			Architecture: fields[2],
			Status:       "installed",
			Size:         size * 1024,
			Description:  desc,
		})
	}
	return packages, nil
}

var aptSimulatedInstallRe = regexp.MustCompile(`^Inst\s+(\S+)\s+\((\S+)\s+(\S+)`)

// ListUpgradable lists packages with an available upgrade. `list --upgradable`
// is an apt-CLI subcommand, so it uses the resolved apt command.
func (a *apt) ListUpgradable(ctx context.Context) ([]PackageUpdate, error) {
	out, err := readOut(ctx, a.r, "apt-get", "-s", "upgrade")
	if err != nil {
		return nil, err
	}

	var updates []PackageUpdate
	for line := range strings.SplitSeq(out, "\n") {
		m := aptSimulatedInstallRe.FindStringSubmatch(line)
		if len(m) < 4 {
			continue
		}
		updates = append(updates, PackageUpdate{
			Name:       m[1],
			NewVersion: m[2],
			Repository: m[3],
		})
	}
	return updates, nil
}

// Show returns detailed information about a package.
func (a *apt) Show(ctx context.Context, name string) (*Package, error) {
	if err := ValidatePackageName(name); err != nil {
		return nil, err
	}
	out, err := readOut(ctx, a.r, "apt-cache", "show", name)
	if err != nil {
		return nil, err
	}

	pkg := &Package{Name: name}
	for line := range strings.SplitSeq(out, "\n") {
		switch {
		case strings.HasPrefix(line, "Version:"):
			pkg.Version = strings.TrimSpace(strings.TrimPrefix(line, "Version:"))
		case strings.HasPrefix(line, "Architecture:"):
			pkg.Architecture = strings.TrimSpace(strings.TrimPrefix(line, "Architecture:"))
		case strings.HasPrefix(line, "Description:"):
			pkg.Description = strings.TrimSpace(strings.TrimPrefix(line, "Description:"))
		case strings.HasPrefix(line, "Installed-Size:"):
			if size, err := strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(line, "Installed-Size:")), 10, 64); err == nil {
				pkg.Size = size * 1024
			}
		}
	}

	installed, err := a.IsInstalled(ctx, name)
	if err != nil {
		return nil, err
	}
	if installed {
		pkg.Status = "installed"
	} else {
		pkg.Status = "available"
	}
	return pkg, nil
}

// ListVersions lists the versions available for a package.
func (a *apt) ListVersions(ctx context.Context, name string) (*VersionInfo, error) {
	if err := ValidatePackageName(name); err != nil {
		return nil, err
	}
	out, err := readOut(ctx, a.r, "apt-cache", "madison", name)
	if err != nil {
		return nil, err
	}

	info := &VersionInfo{Name: name}
	installed, err := a.InstalledVersion(ctx, name)
	if errors.Is(err, ErrNotFound) {
		err = nil
	}
	if err != nil {
		return nil, err
	}
	info.Installed = installed

	seen := make(map[string]bool)
	for line := range strings.SplitSeq(out, "\n") {
		fields := strings.Split(line, "|")
		if len(fields) < 3 {
			continue
		}
		version := strings.TrimSpace(fields[1])
		if seen[version] {
			continue
		}
		seen[version] = true
		info.Versions = append(info.Versions, AvailableVersion{
			Version:    version,
			Repository: strings.TrimSpace(fields[2]),
		})
	}
	return info, nil
}

// LocalPackageInfo reads the canonical Package/Version/Architecture out of a
// local .deb via `dpkg-deb -f` (an unprivileged read). The Package field a
// crafted .deb embeds is untrusted, so it is re-validated with
// ValidatePackageName — the same grammar Remove/IsInstalled would feed it
// to — before being returned; a flag-shaped or metacharacter-bearing name is
// rejected here rather than surfacing as a package-manager flag downstream.
func (a *apt) LocalPackageInfo(ctx context.Context, path string) (*LocalPackage, error) {
	if err := ValidateLocalPackagePath(path); err != nil {
		return nil, err
	}

	out, err := readOut(ctx, a.r, "dpkg-deb", "-f", path, "Package", "Version", "Architecture")
	if err != nil {
		return nil, err
	}
	fields := parseControlFields(out)
	name := fields["Package"]
	if name == "" {
		return nil, fmt.Errorf("pkg: dpkg-deb reported no Package field for %q", path)
	}
	if err := ValidatePackageName(name); err != nil {
		return nil, fmt.Errorf("pkg: local .deb reports an unsafe package name: %w", err)
	}
	return &LocalPackage{Name: name, Version: fields["Version"], Arch: fields["Architecture"]}, nil
}

func parseControlFields(out string) map[string]string {
	fields := make(map[string]string)
	for _, line := range strings.Split(out, "\n") {
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		fields[strings.TrimSpace(key)] = strings.TrimSpace(val)
	}
	return fields
}

// IsInstalled reports whether a package is installed (dpkg -s exits 0).
func (a *apt) IsInstalled(ctx context.Context, name string) (bool, error) {
	if err := ValidatePackageName(name); err != nil {
		return false, err
	}
	res, err := runRead(ctx, a.r, "dpkg", "-s", name)
	if err != nil {
		return false, err
	}
	return res.ExitCode == 0, nil
}

func (a *apt) InstalledVersion(ctx context.Context, name string) (string, error) {
	if err := ValidatePackageName(name); err != nil {
		return "", err
	}
	out, ok, err := probe(ctx, a.r, "dpkg-query", "-W", "-f=${Version}", name)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("apt package %s: %w", name, ErrNotFound)
	}
	return strings.TrimSpace(out), nil
}

// InstalledCount returns the number of installed packages.
func (a *apt) InstalledCount(ctx context.Context) (int, error) {
	out, err := readOut(ctx, a.r, "dpkg-query", "-f", ".\n", "-W")
	if err != nil {
		return 0, err
	}
	return countNonEmptyLines(out), nil
}

func (a *apt) HasUpdates(ctx context.Context) (bool, error) {
	out, err := readOut(ctx, a.r, "apt-get", "-s", "upgrade")
	if err != nil {
		return false, err
	}
	for line := range strings.SplitSeq(out, "\n") {
		if strings.HasPrefix(line, "Inst ") {
			return true, nil
		}
	}
	return false, nil
}

func (a *apt) HasSecurityUpdates(ctx context.Context) (bool, error) {
	bin, err := resolveUnattendedUpgrade()
	if err != nil {
		return false, err
	}
	res, err := runRead(ctx, a.r, bin, "--dry-run", "--verbose")
	if err != nil {
		return false, err
	}
	if res.ExitCode != 0 {
		return false, asCommandError(bin, res)
	}
	return strings.Contains(res.Stdout, "Packages that will be upgraded") || strings.Contains(res.Stdout, "The following packages will be upgraded"), nil
}

// IsPinned reports whether a package is held (apt-mark showhold <name>).
func (a *apt) IsPinned(ctx context.Context, name string) (bool, error) {
	if err := ValidatePackageName(name); err != nil {
		return false, err
	}
	out, err := readOut(ctx, a.r, "apt-mark", "showhold", name)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) == name, nil
}

// ListPinned lists held packages.
func (a *apt) ListPinned(ctx context.Context) ([]Package, error) {
	out, err := readOut(ctx, a.r, "apt-mark", "showhold")
	if err != nil {
		return nil, err
	}

	var packages []Package
	for line := range strings.SplitSeq(out, "\n") {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		version, err := a.InstalledVersion(ctx, name)
		if err != nil {
			return nil, err
		}
		packages = append(packages, Package{
			Name:    name,
			Version: version,
			Status:  "installed",
		})
	}
	return packages, nil
}
