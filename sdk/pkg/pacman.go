package pkg

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
)

var validPacmanPkgName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._+-]*$`)

type pacman struct {
	r sysexec.Runner
}

var _ Manager = (*pacman)(nil)

func (p *pacman) Backend() Backend { return Pacman }

func (p *pacman) write(ctx context.Context, args ...string) (sysexec.Result, error) {
	res, err := runPriv(ctx, p.r, true, nil, "pacman", args...)
	if err != nil {
		return sysexec.Result{}, err
	}
	return res, asCommandError("pacman", res)
}

// Version returns the pacman version string (parsed from " Pacman v6.0.2 - …").
func (p *pacman) Version(ctx context.Context) (string, error) {
	out, err := readOut(ctx, p.r, "pacman", "--version")
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "Pacman v") {
			for _, field := range strings.Fields(line) {
				if strings.HasPrefix(field, "v") {
					return strings.TrimPrefix(field, "v"), nil
				}
			}
		}
	}
	return "", nil
}

func (p *pacman) Install(ctx context.Context, _ InstallOptions, specs ...InstallSpec) (sysexec.Result, error) {
	if len(specs) == 0 {
		return sysexec.Result{}, nil
	}
	args := []string{"-S", "--noconfirm", "--needed"}
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
	return p.write(ctx, args...)
}

// InstallLocal installs a local package file through `pacman -U`, which resolves
// its dependencies from the sync repositories and downgrades naturally when the
// file is older than the installed version — so opts.AllowDowngrade needs no
// extra flag and is ignored. opts.AllowUnsigned is NOT honored either: `pacman
// -U` enforces the repo SigLevel and has no per-invocation signature bypass, so
// the install stays signature-checked (a relaxed SigLevel is a pacman.conf
// concern, out of scope here). ValidateLocalPackagePath requires an absolute
// path, so the operand can never be flag-shaped.
func (p *pacman) InstallLocal(ctx context.Context, path string, _ InstallLocalOptions) (sysexec.Result, error) {
	if err := ValidateLocalPackagePath(path); err != nil {
		return sysexec.Result{}, err
	}
	return p.write(ctx, "-U", "--noconfirm", path)
}

// Remove removes packages; opts.Purge uses -Rns (with deps + config files).
func (p *pacman) Remove(ctx context.Context, opts RemoveOptions, packages ...string) (sysexec.Result, error) {
	if len(packages) == 0 {
		return sysexec.Result{}, nil
	}
	if err := ValidatePackageNames(packages); err != nil {
		return sysexec.Result{}, err
	}
	flag := "-R"
	if opts.Purge {
		flag = "-Rns"
	}
	return p.write(ctx, append([]string{flag, "--noconfirm"}, packages...)...)
}

func (p *pacman) Update(ctx context.Context) (sysexec.Result, error) {
	return sysexec.Result{}, fmt.Errorf("pacman metadata refresh: %w", ErrUnsupported)
}

// Upgrade upgrades the named packages, or the whole system (-Syu) with no names.
func (p *pacman) Upgrade(ctx context.Context, packages ...string) (sysexec.Result, error) {
	if len(packages) == 0 {
		return sysexec.Result{}, nil
	}
	if err := ValidatePackageNames(packages); err != nil {
		return sysexec.Result{}, err
	}
	return p.write(ctx, append([]string{"-S", "--noconfirm"}, packages...)...)
}

func (p *pacman) UpgradeAll(ctx context.Context) (sysexec.Result, error) {
	return p.write(ctx, "-Syu", "--noconfirm")
}

func (p *pacman) UpgradeSecurity(context.Context) (sysexec.Result, error) {
	return sysexec.Result{}, fmt.Errorf("pacman security upgrades: %w", ErrUnsupported)
}

// Autoremove removes orphaned packages (installed as deps, no longer required).
func (p *pacman) Autoremove(ctx context.Context) (sysexec.Result, error) {
	res, err := runRead(ctx, p.r, "pacman", "-Qtdq")
	if err != nil {
		return sysexec.Result{}, err
	}
	if res.ExitCode == 1 {
		return sysexec.Result{}, nil
	}
	if res.ExitCode != 0 {
		return res, asCommandError("pacman", res)
	}
	var orphans []string
	for _, line := range strings.Split(res.Stdout, "\n") {
		if name := strings.TrimSpace(line); name != "" {
			orphans = append(orphans, name)
		}
	}
	if len(orphans) == 0 {
		return sysexec.Result{}, nil
	}
	return p.write(ctx, append([]string{"-Rns", "--noconfirm"}, orphans...)...)
}

// Search searches packages (-Ss; exit 1 = no matches).
func (p *pacman) Search(ctx context.Context, query string) ([]SearchResult, error) {
	if err := ValidateSearchQuery(query); err != nil {
		return nil, err
	}
	res, err := runRead(ctx, p.r, "pacman", "-Ss", query)
	if err != nil {
		return nil, err
	}
	if res.ExitCode == 1 {
		return nil, nil
	}
	if res.ExitCode != 0 {
		return nil, asCommandError("pacman", res)
	}

	var results []SearchResult
	var current *SearchResult
	for line := range strings.SplitSeq(res.Stdout, "\n") {
		if strings.HasPrefix(line, " ") {
			if current != nil {
				current.Description = strings.TrimSpace(line)
				results = append(results, *current)
				current = nil
			}
			continue
		}
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			nameParts := strings.Split(fields[0], "/")
			name := nameParts[len(nameParts)-1]
			repo := ""
			if len(nameParts) > 1 {
				repo = nameParts[0]
			}
			current = &SearchResult{Name: name, Version: fields[1], Repository: repo}
		}
	}
	return results, nil
}

// List lists installed packages (-Q).
func (p *pacman) List(ctx context.Context) ([]Package, error) {
	out, err := readOut(ctx, p.r, "pacman", "-Q")
	if err != nil {
		return nil, err
	}

	var packages []Package
	for line := range strings.SplitSeq(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		packages = append(packages, Package{
			Name:    fields[0],
			Version: fields[1],
			Status:  PackageStatusInstalled,
		})
	}
	return packages, nil
}

// ListUpgradable lists packages with an available upgrade (-Qu; exit 1 = none).
func (p *pacman) ListUpgradable(ctx context.Context) ([]PackageUpdate, error) {
	res, err := runRead(ctx, p.r, "pacman", "-Qu")
	if err != nil {
		return nil, err
	}
	if res.ExitCode == 1 {
		return nil, nil
	}
	if res.ExitCode != 0 {
		return nil, asCommandError("pacman", res)
	}

	var updates []PackageUpdate
	for line := range strings.SplitSeq(res.Stdout, "\n") {
		fields := strings.Fields(line)
		switch {
		case len(fields) >= 4 && fields[2] == "->":
			updates = append(updates, PackageUpdate{
				Name:           fields[0],
				CurrentVersion: fields[1],
				NewVersion:     fields[3],
			})
		case len(fields) >= 2:
			current, err := p.InstalledVersion(ctx, fields[0])
			if err != nil {
				return nil, err
			}
			updates = append(updates, PackageUpdate{
				Name:           fields[0],
				CurrentVersion: current,
				NewVersion:     fields[1],
			})
		}
	}
	return updates, nil
}

// Show returns detailed information about a package (-Qi installed, else -Si).
func (p *pacman) Show(ctx context.Context, name string) (*Package, error) {
	if err := ValidatePackageName(name); err != nil {
		return nil, err
	}

	out, ok, err := probe(ctx, p.r, "pacman", "-Qi", name)
	if err != nil {
		return nil, err
	}
	status := PackageStatusInstalled
	if !ok {
		out, ok, err = probe(ctx, p.r, "pacman", "-Si", name)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("package not found: %s", name)
		}
		status = PackageStatusAvailable
	}

	pkg := &Package{Name: name, Status: status}
	for line := range strings.SplitSeq(out, "\n") {
		switch {
		case strings.HasPrefix(line, "Version"):
			pkg.Version = parseColonValue(line)
		case strings.HasPrefix(line, "Architecture"):
			pkg.Architecture = parseColonValue(line)
		case strings.HasPrefix(line, "Description"):
			pkg.Description = parseColonValue(line)
		case strings.HasPrefix(line, "Installed Size"):

			if n, sizeOK := parsePacmanSize(parseColonValue(line)); sizeOK {
				pkg.Size = n
			}
		case strings.HasPrefix(line, "Repository"):
			pkg.Repository = parseColonValue(line)
		}
	}

	return pkg, nil
}

// ListVersions reports the single repo version pacman keeps for a package.
func (p *pacman) ListVersions(ctx context.Context, name string) (*VersionInfo, error) {
	if err := ValidatePackageName(name); err != nil {
		return nil, err
	}
	info := &VersionInfo{Name: name}
	installed, err := p.InstalledVersion(ctx, name)
	if errors.Is(err, ErrNotFound) {
		err = nil
	}
	if err != nil {
		return nil, err
	}
	info.Installed = installed

	out, ok, err := probe(ctx, p.r, "pacman", "-Si", name)
	if err != nil {
		return nil, err
	}
	if !ok {
		return info, nil
	}

	var version, repo string
	for line := range strings.SplitSeq(out, "\n") {
		switch {
		case strings.HasPrefix(line, "Version"):
			version = parseColonValue(line)
		case strings.HasPrefix(line, "Repository"):
			repo = parseColonValue(line)
		}
	}
	if version != "" {
		info.Versions = append(info.Versions, AvailableVersion{Version: version, Repository: repo})
	}
	return info, nil
}

// LocalPackageInfo reads a local package file's name and version via `pacman -Qp
// <path>` (an unprivileged read), which prints "name version" on one line. The
// name a crafted package embeds is untrusted, so it is re-validated with
// ValidatePackageName before being returned; a flag-shaped or metacharacter-
// bearing first field is rejected. pacman has no architecture in -Qp output, so
// Arch is left empty.
func (p *pacman) LocalPackageInfo(ctx context.Context, path string) (*LocalPackage, error) {
	if err := ValidateLocalPackagePath(path); err != nil {
		return nil, err
	}
	out, err := readOut(ctx, p.r, "pacman", "-Qp", path)
	if err != nil {
		return nil, err
	}

	line := strings.TrimRight(out, "\r\n")
	if strings.TrimSpace(line) == "" || line[0] == ' ' || line[0] == '\t' {
		return nil, fmt.Errorf("pkg: pacman -Qp reported no name for %q", path)
	}
	fields := strings.Fields(line)
	name := fields[0]
	if err := ValidatePackageName(name); err != nil {
		return nil, fmt.Errorf("pkg: local package reports an unsafe name: %w", err)
	}
	info := &LocalPackage{Name: name}
	if len(fields) > 1 {
		info.Version = fields[1]
	}
	return info, nil
}

// IsInstalled reports whether a package is installed (pacman -Q exits 0).
func (p *pacman) IsInstalled(ctx context.Context, name string) (bool, error) {
	if err := ValidatePackageName(name); err != nil {
		return false, err
	}
	res, err := runRead(ctx, p.r, "pacman", "-Q", name)
	if err != nil {
		return false, err
	}
	return res.ExitCode == 0, nil
}

func (p *pacman) InstalledVersion(ctx context.Context, name string) (string, error) {
	if err := ValidatePackageName(name); err != nil {
		return "", err
	}
	res, err := runRead(ctx, p.r, "pacman", "-Q", name)
	if err != nil {
		return "", err
	}
	if res.ExitCode != 0 {
		return "", fmt.Errorf("pacman package %s: %w", name, ErrNotFound)
	}
	fields := strings.Fields(res.Stdout)
	if len(fields) >= 2 {
		return fields[1], nil
	}
	return "", nil
}

// InstalledCount returns the number of installed packages (-Qq).
func (p *pacman) InstalledCount(ctx context.Context) (int, error) {
	out, err := readOut(ctx, p.r, "pacman", "-Qq")
	if err != nil {
		return 0, err
	}
	return countNonEmptyLines(out), nil
}

// HasUpdates reports whether any update is available (-Qu: exit 0 + output).
func (p *pacman) HasUpdates(ctx context.Context) (bool, error) {
	res, err := runRead(ctx, p.r, "pacman", "-Qu")
	if err != nil {
		return false, err
	}
	if res.ExitCode == 1 {
		return false, nil
	}
	if res.ExitCode != 0 {
		return false, asCommandError("pacman", res)
	}
	return strings.TrimSpace(res.Stdout) != "", nil
}

func (p *pacman) HasSecurityUpdates(context.Context) (bool, error) {
	return false, fmt.Errorf("pacman security updates: %w", ErrUnsupported)
}

// Pin holds packages by adding them to IgnorePkg in /etc/pacman.conf. Pinning is
// a config-file edit, not a package transaction, so it has no command output to
// surface — the returned Result is the zero Result.
func (p *pacman) Pin(ctx context.Context, packages ...string) (sysexec.Result, error) {
	if len(packages) == 0 {
		return sysexec.Result{}, nil
	}
	if err := ValidatePackageNames(packages); err != nil {
		return sysexec.Result{}, err
	}

	for _, name := range packages {
		if !validPacmanPkgName.MatchString(name) {
			return sysexec.Result{}, fmt.Errorf("%w: invalid package name %q: must match [a-zA-Z0-9][a-zA-Z0-9._+-]*", ErrInvalidArgument, name)
		}
	}

	conf, err := p.readConf(ctx)
	if err != nil {
		return sysexec.Result{}, err
	}
	ignored := getIgnoredPackages(conf)
	for _, name := range packages {
		if !slices.Contains(ignored, name) {
			ignored = append(ignored, name)
		}
	}
	return sysexec.Result{}, p.writeIgnorePkg(ctx, conf, ignored)
}

// Unpin releases packages by removing them from IgnorePkg. Like Pin, this is a
// config-file edit with no command output (zero Result).
func (p *pacman) Unpin(ctx context.Context, packages ...string) (sysexec.Result, error) {
	if len(packages) == 0 {
		return sysexec.Result{}, nil
	}
	if err := ValidatePackageNames(packages); err != nil {
		return sysexec.Result{}, err
	}
	conf, err := p.readConf(ctx)
	if err != nil {
		return sysexec.Result{}, err
	}
	var kept []string
	for _, name := range getIgnoredPackages(conf) {
		if !slices.Contains(packages, name) {
			kept = append(kept, name)
		}
	}
	return sysexec.Result{}, p.writeIgnorePkg(ctx, conf, kept)
}

// ListPinned lists IgnorePkg-held packages.
func (p *pacman) ListPinned(ctx context.Context) ([]Package, error) {
	conf, err := p.readConf(ctx)
	if err != nil {
		return nil, err
	}
	var packages []Package
	for _, name := range getIgnoredPackages(conf) {
		version, err := p.InstalledVersion(ctx, name)
		if err != nil {
			return nil, err
		}
		packages = append(packages, Package{
			Name:    name,
			Version: version,
			Status:  PackageStatusInstalled,
		})
	}
	return packages, nil
}

// IsPinned reports whether a package is in IgnorePkg.
func (p *pacman) IsPinned(ctx context.Context, name string) (bool, error) {
	if err := ValidatePackageName(name); err != nil {
		return false, err
	}
	conf, err := p.readConf(ctx)
	if err != nil {
		return false, err
	}
	return slices.Contains(getIgnoredPackages(conf), name), nil
}

func (p *pacman) readConf(ctx context.Context) (string, error) {
	out, err := readOut(ctx, p.r, "cat", "/etc/pacman.conf")
	if err != nil {
		return "", fmt.Errorf("failed to read pacman.conf: %w", err)
	}
	return out, nil
}

func (p *pacman) writeIgnorePkg(ctx context.Context, conf string, ignored []string) error {
	out := buildIgnorePkgConf(conf, ignored)
	res, err := runPrivStdin(ctx, p.r, true, nil, out, "tee", "/etc/pacman.conf")
	if err != nil {
		return err
	}
	return asCommandError("tee", res)
}

func getIgnoredPackages(conf string) []string {
	var ignored []string
	for line := range strings.SplitSeq(conf, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "IgnorePkg") {
			continue
		}
		if parts := strings.SplitN(line, "=", 2); len(parts) == 2 {
			ignored = append(ignored, strings.Fields(strings.TrimSpace(parts[1]))...)
		}
	}
	return ignored
}

func buildIgnorePkgConf(conf string, ignored []string) string {
	var b strings.Builder
	found := false
	for line := range strings.SplitSeq(conf, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "IgnorePkg") {

			if !found {
				found = true
				if len(ignored) > 0 {
					fmt.Fprintf(&b, "IgnorePkg = %s\n", strings.Join(ignored, " "))
				}
			}
			continue
		}
		b.WriteString(line + "\n")
	}

	if !found && len(ignored) > 0 {
		content := b.String()
		if optionsIdx := strings.Index(content, "[options]"); optionsIdx != -1 {
			if nl := strings.Index(content[optionsIdx:], "\n"); nl != -1 {
				insert := optionsIdx + nl + 1
				content = content[:insert] + fmt.Sprintf("IgnorePkg = %s\n", strings.Join(ignored, " ")) + content[insert:]
			}
		}
		b.Reset()
		b.WriteString(content)
	}
	return b.String()
}

func parsePacmanSize(s string) (int64, bool) {
	return parseSizeWithUnits(s, []sizeUnit{
		{" KiB", 1024},
		{" MiB", 1024 * 1024},
		{" GiB", 1024 * 1024 * 1024},
		{" B", 1},
	})
}
