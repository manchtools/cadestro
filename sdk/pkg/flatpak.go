package pkg

import (
	"context"
	"errors"
	"fmt"
	"strings"

	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
)

// FlatpakManager drives Flatpak over an injected runner.
type FlatpakManager struct {
	r      sysexec.Runner
	system bool
}

// NewFlatpak creates a system-scope manager.
func NewFlatpak(runner sysexec.Runner) (*FlatpakManager, error) {
	if runner == nil {
		return nil, fmt.Errorf("flatpak runner: %w", sysexec.ErrRunnerRequired)
	}
	return &FlatpakManager{r: runner, system: true}, nil
}

// NewUserFlatpak creates a per-user manager.
func NewUserFlatpak(runner sysexec.Runner) (*FlatpakManager, error) {
	if runner == nil {
		return nil, fmt.Errorf("flatpak runner: %w", sysexec.ErrRunnerRequired)
	}
	return &FlatpakManager{r: runner}, nil
}

// FlatpakAvailable reports whether the Flatpak binary is installed.
func FlatpakAvailable() bool {
	_, err := lookPath("flatpak")
	return err == nil
}

func (f *FlatpakManager) scope() string {
	if f.system {
		return "--system"
	}
	return "--user"
}

func (f *FlatpakManager) write(ctx context.Context, args ...string) (sysexec.Result, error) {
	res, err := runPriv(ctx, f.r, f.system, nil, "flatpak", args...)
	if err != nil {
		return sysexec.Result{}, err
	}
	return res, asCommandError("flatpak", res)
}

// Version returns the flatpak version string ("Flatpak 1.14.4").
func (f *FlatpakManager) Version(ctx context.Context) (string, error) {
	out, err := readOut(ctx, f.r, "flatpak", "--version")
	if err != nil {
		return "", err
	}
	parts := strings.Fields(out)
	if len(parts) >= 2 {
		return parts[1], nil
	}
	return "", nil
}

// Install installs refs from an explicit remote.
func (f *FlatpakManager) Install(ctx context.Context, remote string, refs ...string) (sysexec.Result, error) {
	if len(refs) == 0 {
		return sysexec.Result{}, nil
	}
	if err := ValidateRemoteName(remote); err != nil {
		return sysexec.Result{}, err
	}
	if err := ValidatePackageNames(refs); err != nil {
		return sysexec.Result{}, err
	}
	args := []string{"install", "-y", "--noninteractive", f.scope()}
	args = append(args, remote)
	args = append(args, refs...)
	return f.write(ctx, args...)
}

// InstallLocal installs a local flatpak bundle (a single-file .flatpak) or a
// .flatpakref. A bundle has no version-ordering concept, so opts.AllowDowngrade
// is ignored; opts.AllowUnsigned is a no-op (a bundle's signing is not a
// per-file GPG check). System scope escalates, --user does not.
// ValidateLocalPackagePath requires an absolute path, so the operand can never
// be flag-shaped.
func (f *FlatpakManager) InstallLocal(ctx context.Context, path string, _ InstallLocalOptions) (sysexec.Result, error) {
	if err := ValidateLocalPackagePath(path); err != nil {
		return sysexec.Result{}, err
	}
	flags := []string{"install", "-y", "--noninteractive", f.scope()}
	return f.write(ctx, append(flags, path)...)
}

// Remove uninstalls bundles; opts.Purge also deletes per-app data (--delete-data).
func (f *FlatpakManager) Remove(ctx context.Context, opts RemoveOptions, packages ...string) (sysexec.Result, error) {
	if len(packages) == 0 {
		return sysexec.Result{}, nil
	}
	if err := ValidatePackageNames(packages); err != nil {
		return sysexec.Result{}, err
	}
	args := []string{"uninstall", "-y", "--noninteractive"}
	if opts.Purge {
		args = append(args, "--delete-data")
	}
	args = append(args, f.scope())
	args = append(args, packages...)
	return f.write(ctx, args...)
}

// Update refreshes appstream metadata for the configured remotes.
func (f *FlatpakManager) Update(ctx context.Context) (sysexec.Result, error) {
	return f.write(ctx, "update", "--appstream", "-y", "--noninteractive", f.scope())
}

// Upgrade updates the named bundles, or all installed bundles with no names.
func (f *FlatpakManager) Upgrade(ctx context.Context, packages ...string) (sysexec.Result, error) {
	if len(packages) == 0 {
		return sysexec.Result{}, nil
	}
	if err := ValidatePackageNames(packages); err != nil {
		return sysexec.Result{}, err
	}
	args := append([]string{"update", "-y", "--noninteractive", f.scope()}, packages...)
	return f.write(ctx, args...)
}

// UpgradeAll updates every installed app/runtime (flatpak update with no refs).
func (f *FlatpakManager) UpgradeAll(ctx context.Context) (sysexec.Result, error) {
	return f.write(ctx, "update", "-y", "--noninteractive", f.scope())
}

func (f *FlatpakManager) HasSecurityUpdates(context.Context) (bool, error) {
	return false, fmt.Errorf("flatpak security updates: %w", ErrUnsupported)
}

// Autoremove removes unused runtimes/extensions (flatpak uninstall --unused).
func (f *FlatpakManager) Autoremove(ctx context.Context) (sysexec.Result, error) {
	return f.write(ctx, "uninstall", "--unused", "-y", "--noninteractive", f.scope())
}

// Search searches configured remotes (exit 1 = no matches).
func (f *FlatpakManager) Search(ctx context.Context, query string) ([]SearchResult, error) {
	if err := ValidateSearchQuery(query); err != nil {
		return nil, err
	}
	res, err := runRead(ctx, f.r, "flatpak", "search", query)
	if err != nil {
		return nil, err
	}
	if res.ExitCode == 1 {
		return nil, nil
	}
	if res.ExitCode != 0 {
		return nil, asCommandError("flatpak", res)
	}

	var results []SearchResult
	lines := strings.SplitSeq(res.Stdout, "\n")
	first := ""
	firstLine := true
	for line := range lines {
		if !firstLine {
			if r := parseFlatpakSearchLine(line); r != nil {
				results = append(results, *r)
			}
			continue
		}
		firstLine = false
		first = line
		if strings.Contains(first, "\t") {
			if r := parseFlatpakSearchLine(first); r != nil {
				results = append(results, *r)
			}
		}
	}
	return results, nil
}

// List lists installed application bundles.
func (f *FlatpakManager) List(ctx context.Context) ([]Package, error) {
	out, err := readOut(ctx, f.r, "flatpak", "list",
		"--columns=application,version,arch,size,description,origin", f.scope())
	if err != nil {
		return nil, err
	}

	var packages []Package
	for line := range strings.SplitSeq(out, "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) < 4 {
			continue
		}
		desc := ""
		if len(fields) > 4 {
			desc = fields[4]
		}
		repo := ""
		if len(fields) > 5 {
			repo = fields[5]
		}

		var size int64
		if n, sizeOK := parseFlatpakSize(fields[3]); sizeOK {
			size = n
		}
		packages = append(packages, Package{
			Name:         fields[0],
			Version:      fields[1],
			Architecture: fields[2],
			Status:       "installed",
			Size:         size,
			Description:  desc,
			Repository:   repo,
		})
	}
	return packages, nil
}

// ListUpgradable lists bundles with an available update.
func (f *FlatpakManager) ListUpgradable(ctx context.Context) ([]PackageUpdate, error) {
	out, err := readOut(ctx, f.r, "flatpak", "remote-ls", "--updates",
		"--columns=application,version,origin", f.scope())
	if err != nil {
		return nil, err
	}

	var updates []PackageUpdate
	for line := range strings.SplitSeq(out, "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) < 2 {
			continue
		}
		repo := ""
		if len(fields) > 2 {
			repo = fields[2]
		}
		current, err := f.InstalledVersion(ctx, fields[0])
		if err != nil {
			return nil, err
		}
		updates = append(updates, PackageUpdate{
			Name:           fields[0],
			NewVersion:     fields[1],
			CurrentVersion: current,
			Repository:     repo,
		})
	}
	return updates, nil
}

// Show returns detailed information about a bundle, falling back to the flathub
// remote when the bundle is not installed.
func (f *FlatpakManager) Show(ctx context.Context, name string) (*Package, error) {
	if err := ValidatePackageName(name); err != nil {
		return nil, err
	}

	out, ok, err := probe(ctx, f.r, "flatpak", "info", name, f.scope())
	if err != nil {
		return nil, err
	}
	if !ok {
		return f.showFromRemote(ctx, name)
	}

	pkg := &Package{Name: name, Status: "installed"}
	for line := range strings.SplitSeq(out, "\n") {
		switch {
		case strings.HasPrefix(line, "Version:"):
			pkg.Version = parseFlatpakValue(line)
		case strings.HasPrefix(line, "Arch:"):
			pkg.Architecture = parseFlatpakValue(line)
		case strings.HasPrefix(line, "Description:"):
			pkg.Description = parseFlatpakValue(line)
		case strings.HasPrefix(line, "Installed:"), strings.HasPrefix(line, "Size:"):

			if n, sizeOK := parseFlatpakSize(parseFlatpakValue(line)); sizeOK {
				pkg.Size = n
			}
		case strings.HasPrefix(line, "Origin:"):
			pkg.Repository = parseFlatpakValue(line)
		}
	}

	return pkg, nil
}

func (f *FlatpakManager) showFromRemote(ctx context.Context, name string) (*Package, error) {

	out, ok, err := probe(ctx, f.r, "flatpak", "remote-info", "flathub", name)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("package not found: %s", name)
	}

	pkg := &Package{Name: name, Status: "available", Repository: "flathub"}
	for line := range strings.SplitSeq(out, "\n") {
		switch {
		case strings.HasPrefix(line, "Version:"):
			pkg.Version = parseFlatpakValue(line)
		case strings.HasPrefix(line, "Arch:"):
			pkg.Architecture = parseFlatpakValue(line)
		case strings.HasPrefix(line, "Description:"):
			pkg.Description = parseFlatpakValue(line)
		case strings.HasPrefix(line, "Download:"), strings.HasPrefix(line, "Size:"):

			if n, sizeOK := parseFlatpakSize(parseFlatpakValue(line)); sizeOK {
				pkg.Size = n
			}
		}
	}
	return pkg, nil
}

// ListVersions reports the single remote (flathub) version for a bundle.
func (f *FlatpakManager) ListVersions(ctx context.Context, name string) (*VersionInfo, error) {
	if err := ValidatePackageName(name); err != nil {
		return nil, err
	}
	info := &VersionInfo{Name: name}
	installed, err := f.InstalledVersion(ctx, name)
	if errors.Is(err, ErrNotFound) {
		err = nil
	}
	if err != nil {
		return nil, err
	}
	info.Installed = installed

	out, ok, err := probe(ctx, f.r, "flatpak", "remote-info", "flathub", name)
	if err != nil {
		return nil, err
	}
	if !ok {
		return info, nil
	}
	for line := range strings.SplitSeq(out, "\n") {
		if strings.HasPrefix(line, "Version:") {
			info.Versions = append(info.Versions, AvailableVersion{
				Version:    parseFlatpakValue(line),
				Repository: "flathub",
			})
			break
		}
	}
	return info, nil
}

// LocalPackageInfo is not supported for flatpak: a .flatpak bundle has no clean,
// non-installing name-introspection command (its ref must be trusted from the
// bundle metadata, which `flatpak install` itself reads), so rather than guess a
// name from an attacker-influenced bundle this fails closed with a clear error.
func (f *FlatpakManager) LocalPackageInfo(_ context.Context, _ string) (*LocalPackage, error) {
	return nil, fmt.Errorf("flatpak local package info: %w", ErrUnsupported)
}

// IsInstalled reports whether a bundle is installed (flatpak info exits 0).
func (f *FlatpakManager) IsInstalled(ctx context.Context, name string) (bool, error) {
	if err := ValidatePackageName(name); err != nil {
		return false, err
	}
	res, err := runRead(ctx, f.r, "flatpak", "info", name, f.scope())
	if err != nil {
		return false, err
	}
	return res.ExitCode == 0, nil
}

// InstalledVersion returns the installed version of a bundle, or ErrNotFound.
func (f *FlatpakManager) InstalledVersion(ctx context.Context, name string) (string, error) {
	if err := ValidatePackageName(name); err != nil {
		return "", err
	}
	res, err := runRead(ctx, f.r, "flatpak", "info", "--show-version", name, f.scope())
	if err != nil {
		return "", err
	}
	if res.ExitCode != 0 {
		return "", fmt.Errorf("flatpak package %s: %w", name, ErrNotFound)
	}
	return strings.TrimSpace(res.Stdout), nil
}

// InstalledCount returns the number of installed bundles.
func (f *FlatpakManager) InstalledCount(ctx context.Context) (int, error) {
	out, err := readOut(ctx, f.r, "flatpak", "list", "--columns=application", f.scope())
	if err != nil {
		return 0, err
	}
	return countNonEmptyLines(out), nil
}

// HasUpdates reports whether any bundle has an available update.
func (f *FlatpakManager) HasUpdates(ctx context.Context) (bool, error) {
	out, err := readOut(ctx, f.r, "flatpak", "remote-ls", "--updates", "--columns=application", f.scope())
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

// Pin masks bundles so they are held back from updates. Best-effort across the
// set: every bundle is attempted and the last error (if any) is returned, along
// with the last command's Result.
func (f *FlatpakManager) Pin(ctx context.Context, packages ...string) (sysexec.Result, error) {
	if len(packages) == 0 {
		return sysexec.Result{}, nil
	}
	if err := ValidatePackageNames(packages); err != nil {
		return sysexec.Result{}, err
	}
	var last sysexec.Result
	var lastErr error
	for _, name := range packages {
		res, err := f.write(ctx, "mask", name, f.scope())
		last = res
		if err != nil {
			lastErr = err
		}
	}
	return last, lastErr
}

// Unpin removes the mask so bundles update again.
func (f *FlatpakManager) Unpin(ctx context.Context, packages ...string) (sysexec.Result, error) {
	if len(packages) == 0 {
		return sysexec.Result{}, nil
	}
	if err := ValidatePackageNames(packages); err != nil {
		return sysexec.Result{}, err
	}
	var last sysexec.Result
	var lastErr error
	for _, name := range packages {
		res, err := f.write(ctx, "mask", "--remove", name, f.scope())
		last = res
		if err != nil {
			lastErr = err
		}
	}
	return last, lastErr
}

// ListPinned lists masked bundles.
func (f *FlatpakManager) ListPinned(ctx context.Context) ([]Package, error) {
	out, err := readOut(ctx, f.r, "flatpak", "mask", f.scope())
	if err != nil {
		return nil, err
	}

	var packages []Package
	for line := range strings.SplitSeq(out, "\n") {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		version, err := f.InstalledVersion(ctx, name)
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

// IsPinned reports whether a bundle is masked.
func (f *FlatpakManager) IsPinned(ctx context.Context, name string) (bool, error) {
	if err := ValidatePackageName(name); err != nil {
		return false, err
	}
	out, ok, err := probe(ctx, f.r, "flatpak", "mask", f.scope())
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	for line := range strings.SplitSeq(out, "\n") {
		if strings.TrimSpace(line) == name {
			return true, nil
		}
	}
	return false, nil
}

// AddRemote registers a flatpak remote. name must be a valid remote alias and
// url an https repository URL (validated to keep flag/metacharacter and
// plaintext-transport inputs off the argv and out of the trust path).
func (f *FlatpakManager) AddRemote(ctx context.Context, name, url string) error {
	if err := ValidateRemoteName(name); err != nil {
		return err
	}
	if err := ValidateRepoBaseURL(url); err != nil {
		return err
	}

	_, err := f.write(ctx, "remote-add", "--if-not-exists", name, url, f.scope())
	return err
}

// RemoveRemote deletes a flatpak remote.
func (f *FlatpakManager) RemoveRemote(ctx context.Context, name string) error {
	if err := ValidateRemoteName(name); err != nil {
		return err
	}
	_, err := f.write(ctx, "remote-delete", "--force", name, f.scope())
	return err
}

// ListRemotes returns the configured flatpak remote names.
func (f *FlatpakManager) ListRemotes(ctx context.Context) ([]string, error) {
	out, err := readOut(ctx, f.r, "flatpak", "remotes", "--columns=name", f.scope())
	if err != nil {
		return nil, err
	}
	var remotes []string
	for line := range strings.SplitSeq(out, "\n") {
		if name := strings.TrimSpace(line); name != "" {
			remotes = append(remotes, name)
		}
	}
	return remotes, nil
}

func parseFlatpakSearchLine(line string) *SearchResult {

	fields := strings.Split(line, "\t")
	if len(fields) < 3 {
		return nil
	}
	return &SearchResult{
		Name:        fields[2],
		Description: fields[1],
	}
}

func parseFlatpakValue(line string) string {
	parts := strings.SplitN(line, ":", 2)
	if len(parts) < 2 {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func parseFlatpakSize(s string) (int64, bool) {

	s = strings.ReplaceAll(s, ",", "")
	return parseSizeWithUnits(s, []sizeUnit{
		{" kB", 1000},
		{" KB", 1000},
		{" KiB", 1024},
		{" MB", 1000 * 1000},
		{" MiB", 1024 * 1024},
		{" GB", 1000 * 1000 * 1000},
		{" GiB", 1024 * 1024 * 1024},
		{" bytes", 1},
	})
}
