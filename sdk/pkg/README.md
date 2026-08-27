# Package Manager SDK

A Go library for driving native Linux package managers (apt, dnf/dnf5, pacman,
zypper) through a single `Manager` interface, with a separate Flatpak manager,
built over an injected
`exec.Runner` — no global escalation state, fully unit-testable with
`exectest.FakeRunner` (no host, no sudo, no container).

## Installation

```go
import (
    "github.com/manchtools/cadestro/sdk/pkg"
    "github.com/manchtools/cadestro/sdk/sys/exec"
)
```

## Quick Start

```go
// Build a Runner for the host's escalation backend, then a Manager for a backend.
r, err := exec.NewRunner(exec.Sudo) // or exec.Doas / exec.Direct (already root)
if err != nil {
    log.Fatal(err)
}
m, err := pkg.New(pkg.Apt, r)
if err != nil {
    log.Fatal(err)
}

ctx := context.Background()

// Install latest packages.
_, err = m.Install(ctx, pkg.InstallOptions{}, pkg.InstallSpec{Name: "nginx"}, pkg.InstallSpec{Name: "curl"})

// Each package carries its own optional version.
_, err = m.Install(ctx, pkg.InstallOptions{}, pkg.InstallSpec{Name: "nginx", Version: "1.24.0-1"})

// Allow a downgrade where the backend supports it.
_, err = m.Install(ctx, pkg.InstallOptions{AllowDowngrade: true}, pkg.InstallSpec{Name: "nginx", Version: "1.22.0-1"})
```

Mutating methods return the command result and an error — a non-zero exit becomes an
`*exec.CommandError` carrying the exit code and stderr (`errors.As` to inspect).
Query methods return typed results.

## Choosing a backend

`New` is pure — it does not probe the host. Use `Detect` to learn which
backends are installed (it lists, in priority order; it never picks):

```go
for _, b := range pkg.Detect() {
    fmt.Println(b) // "apt", "dnf5", ...
}
m, err := pkg.New(pkg.Dnf, r)
```

An unknown backend or a nil runner is rejected (`pkg.ErrUnknownBackend` /
"runner is required") — fail-closed, no silent default.

## Mutations

```go
m.Install(ctx, pkg.InstallOptions{}, pkg.InstallSpec{Name: "nginx"})
m.InstallLocal(ctx, "/var/cache/pm/app.deb", pkg.InstallLocalOptions{})
m.Remove(ctx, pkg.RemoveOptions{}, "nginx")
m.Remove(ctx, pkg.RemoveOptions{Purge: true}, "nginx")
m.Update(ctx)
m.UpgradeAll(ctx)
m.Upgrade(ctx, "nginx", "curl")
m.Pin(ctx, "nginx")
m.Unpin(ctx, "nginx")
m.Autoremove(ctx)
```

Reads run unprivileged; mutations escalate through the Runner's backend
(`sudo -n` / `doas -n` / bare for `Direct`).

## Queries

```go
ver, _ := m.Version(ctx)                 // package-manager tool version
packages, _ := m.List(ctx)               // installed packages
updates, _ := m.ListUpgradable(ctx)      // []PackageUpdate
p, _ := m.Show(ctx, "nginx")             // *Package
info, _ := m.ListVersions(ctx, "nginx")  // *VersionInfo
ok, _ := m.IsInstalled(ctx, "nginx")
v, _ := m.InstalledVersion(ctx, "nginx") // ErrNotFound if absent
n, _ := m.InstalledCount(ctx)
has, _ := m.HasUpdates(ctx)             // true if any update
security, _ := m.HasSecurityUpdates(ctx) // true if security updates exist
pinned, _ := m.IsPinned(ctx, "nginx")
held, _ := m.ListPinned(ctx)
results, _ := m.Search(ctx, "nginx")
```

## Flatpak

Flatpak has a separate concrete manager because it is not a native distro
backend. Use the system or per-user constructor explicitly:

```go
m, _ := pkg.NewFlatpak(r)
mu, _ := pkg.NewUserFlatpak(r)
m.AddRemote(ctx, "flathub", "https://dl.flathub.org/repo/flathub.flatpakrepo")
remotes, _ := m.ListRemotes(ctx)
_, _ = mu.List(ctx)
```

`AddRemote` validates the alias (`ValidateRemoteName`) and the URL
(`ValidateRepoBaseURL`, https only). Flatpak installs use application IDs and
an explicit remote; it does not use native package versions.

## Types

```go
type InstallOptions struct {
    AllowDowngrade bool
}

type InstallLocalOptions struct {
    AllowDowngrade bool // install a file older than the installed version
    AllowUnsigned  bool // skip the backend GPG check (dnf/dnf5 --nogpgcheck / zypper
                        // --allow-unsigned-rpm) for an out-of-band-verified file;
                        // secure-default-off. apt/flatpak: no-op; pacman: NOT
                        // honored (pacman -U always enforces SigLevel).
}

type RemoveOptions struct {
    Purge bool // also remove config/data (apt purge / pacman -Rns / flatpak --delete-data)
}

type Package struct {
    Name, Version, Architecture, Description, Status, Repository string
    Size                                                         int64 // bytes
}

type PackageUpdate struct {
    Name, CurrentVersion, NewVersion, Architecture, Repository string
}

type VersionInfo struct {
    Name      string
    Versions  []AvailableVersion
    Installed string
}
```

## Supported Package Managers

| Backend | Systems | `Detect` binary | Pinning |
|---------|---------|-----------------|---------|
| `pkg.Apt` | Debian, Ubuntu, Mint | `apt-get` | `apt-mark hold/unhold` |
| `pkg.Dnf` | Fedora, RHEL 8+, CentOS Stream | `dnf` | `dnf versionlock` when available |
| `pkg.Dnf5` | Fedora, RHEL, CentOS Stream with dnf5 | `dnf5` | `dnf5 versionlock` when available |
| `pkg.Pacman` | Arch, Manjaro | `pacman` | `IgnorePkg` in `/etc/pacman.conf` |
| `pkg.Zypper` | openSUSE, SLES | `zypper` | `zypper addlock/removelock` |

## Argument-Hardening Validators

Every value that reaches a package-manager `argv` is validated against its
*intent* before the command runs. Package names and versions are checked inside
each method (there is no opt-out). The remaining exported validators are
**mandatory** at the argv boundaries they protect (the agent's executors call
them, and positionals are passed after an explicit `--` end-of-options separator
built with `exec.SeparatePositionals`, so a flag-shaped value can never be
reparsed as an option):

| Validator | Guards | Rule |
|-----------|--------|------|
| `ValidatePackageName` / `ValidatePackageNames` | apt/dnf/dnf5/pacman/zypper/flatpak names | first char alphanumeric, then `[a-zA-Z0-9._+:/@~-]`, ≤256 |
| `ValidatePackageVersion` | `<name>=<version>` argv | cross-distro EVR grammar, empty = "no pin" |
| `ValidateRpmPackageName` | `rpm -q` / `rpm -e <NAME>` (NAME read off a crafted `.rpm`) | first char alphanumeric, then `[a-zA-Z0-9._+-]`, ≤256 |
| `ValidateRepoBaseURL` | dnf/dnf5 `baseurl` / zypper `url` / pacman `server` / flatpak remote URL | **https only**, host required, control-char free (template vars `$releasever`/`$arch` allowed). apt is excluded — its security is the gpg-signed Release file. |
| `ValidateGpgKeyRef` | dnf/dnf5/zypper `gpgkey` passed to `rpm --import` | https URL, `file:///abs` path, or absolute path; no `..`, no leading `-`, no `http://`, no `ext::` |
| `ValidateRemoteName` | flatpak remote alias | first char alphanumeric, then `[a-zA-Z0-9._-]`, ≤128 |
| `ValidateLocalPackagePath` | `InstallLocal` file operand (`apt-get`/`dnf`/`dnf5`/`pacman -U`/`zypper`/`flatpak` install) | **absolute path** (never flag-shaped), no `..`, no control chars; a space is allowed |
| `ValidateSearchQuery` | the free-text `Search` query | must not start with `-` (a search term cannot be `--`-guarded because dnf5 rejects `--`), no control chars; empty allowed |

In addition, pacman's `Pin` runs a stricter `[a-zA-Z0-9][a-zA-Z0-9._+-]*` gate
before a name is written to `IgnorePkg`, blocking config-injection even for
names that `ValidatePackageName` would accept.

A self-discovering test (`TestEveryManagerMethodNeutralizesFlagShapedOperands`)
reflects over the whole `Manager` surface and fails if any method lets a
flag-shaped operand reach `argv` as an option, so a new method that forgets to
validate its operand cannot pass CI.

## Testing

Because the Manager is built with an injected Runner, tests pass an
`exectest.FakeRunner`, script command results, and assert on the exact
`exec.Command`s the Manager built (argv, escalation, stdin) — no real package
manager is invoked:

```go
f := exectest.New(exec.Direct)
f.Push(exec.Result{Stdout: "..."}, nil)
m, _ := pkg.New(pkg.Apt, f)
m.Install(ctx, pkg.InstallOptions{}, pkg.InstallSpec{Name: "nginx"})
// f.Calls()[0] is the recorded `apt install -y --fix-broken nginx`
```

## Notes

- **Non-interactive mode**: apt commands run with
  `DEBIAN_FRONTEND=noninteractive`; the C locale (`LANG=C`/`LC_ALL=C`) is forced
  on every command for stable English-only output parsing.
- **Version formats**: apt `1.24.0-1ubuntu1`, dnf/dnf5 `1.24.0-1.fc39`, pacman
  `1.24.0-1`, zypper `1.24.0-1.1`. Flatpak addresses bundles by application ID
  (e.g. `org.mozilla.firefox`) and has no version pin.
- **Pinning setup**: dnf/dnf5 use their installed versionlock command when available;
  pacman edits `/etc/pacman.conf` (root). apt/zypper need no setup.
