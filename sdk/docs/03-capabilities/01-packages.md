---
title: Packages
label: Packages
description: Install, remove, upgrade, and query software across native Linux package managers, with a separate Flatpak manager.
icon: "📦"
---

# Packages

`pkg` manages software through the host's native package manager behind a
single `Manager` interface. One set of calls drives apt, dnf/dnf5, pacman, or
zypper. Flatpak has a separate concrete manager with explicit remotes.

It follows the [architecture](/concepts/architecture): build a Runner, choose a
[Backend](/concepts/backends), get a Manager.

## Construct a manager

`pkg.Detect` reports which package managers are actually installed, so a caller
can pick one instead of guessing:

```go
r, err := exec.NewRunner(exec.Sudo) // package mutations need root
if err != nil {
    return err
}

backends := pkg.Detect() // e.g. [apt] on Debian, [dnf] on Fedora
if len(backends) == 0 {
    return errors.New("no supported package manager on this host")
}

m, err := pkg.New(backends[0], r)
if err != nil {
    return err
}
```

<!-- docref: begin src=pkg/pkg.go#Detect:8927e775 -->
`Detect` lists the backends whose tool is present on `PATH`; it never picks one
and never constructs a Manager — the caller reads the list, chooses explicitly,
and passes that choice to `New`. There is no hidden auto-detection.
<!-- docref: end -->

<!-- docref: begin src=pkg/pkg.go#New:2be34fca -->
`New` is pure and fail-closed: it validates the backend and rejects a nil
Runner before returning a Manager, and it does **not** probe the host. A
zero-value or unimplemented backend is an error, not a silent default — so a
misconfigured caller fails loudly at construction rather than mid-operation.
<!-- docref: end -->

## Install, remove, upgrade

Every mutation returns the command's `exec.Result` (exit code, stdout, stderr)
so a caller can surface exactly what the package manager did:

```go
res, err := m.Install(ctx, pkg.InstallOptions{}, pkg.InstallSpec{Name: "vim"}, pkg.InstallSpec{Name: "git"})
if err != nil {
    return fmt.Errorf("install failed: %w", err)
}
fmt.Println(res.Stdout)

if _, err := m.Remove(ctx, pkg.RemoveOptions{}, "telnet"); err != nil {
    return err
}

// Refresh the index first on backends that need it (pacman's UpgradeAll
// already syncs in-transaction), then upgrade everything. A failed
// refresh must not fall through to the upgrade:
if _, err := m.Update(ctx); err != nil {
    return err
}
if _, err := m.UpgradeAll(ctx); err != nil {
    return err
}

// Or upgrade specific packages:
if _, err := m.Upgrade(ctx, "openssl"); err != nil {
    return err
}

// Drop orphaned dependencies:
if _, err := m.Autoremove(ctx); err != nil {
    return err
}
```

<!-- docref: begin src=pkg/dnf.go#dnf.Install:b9bab404 -->
Package specifications are passed as validated operands, and every mutation
returns the package manager's `exec.Result`.
<!-- docref: end -->

## Query installed state

<!-- docref: begin src=sys/exec/runner.go#Direct:ed029c0e,pkg/exec.go#runRead:50c48e99 -->
Reads never escalate — the query path runs each command without the privilege
wrapper, so a `Direct` Runner is enough:
<!-- docref: end -->

```go
ok, err := m.IsInstalled(ctx, "curl")
ver, err := m.InstalledVersion(ctx, "curl") // ErrNotFound when absent
n, err := m.InstalledCount(ctx)             // total installed packages
```

<!-- docref: begin src=pkg/dnf.go#dnf.IsInstalled:28231b96,pkg/dnf.go#dnf.InstalledVersion:01a1110e -->
The queries are unprivileged: `IsInstalled` reports whether a package is present,
and `InstalledVersion` returns its version or an `ErrNotFound` error.
<!-- docref: end -->

## Backends

<!-- docref: begin src=pkg/pkg.go#Backend:11393461 -->
The backend is fixed at construction and selected from apt, dnf/dnf5, pacman, or
zypper. Flatpak has its own concrete manager. The zero value is invalid; only
implemented backends exist, so there is no "unknown backend" that silently does
nothing.
<!-- docref: end -->

Behavioural differences the Manager smooths over but you should know about:

<!-- docref: begin src=pkg/pkg.go#Manager:3b22a029,pkg/pacman.go#pacman.UpgradeAll:3da8f270 -->
- `Update` is the explicit index refresh; `UpgradeAll` maps to the backend's
  full upgrade (`apt upgrade` / `dnf/dnf5 upgrade` / `zypper update`) and does
  **not** re-sync the index first — except
  **pacman**, whose `-Syu` syncs the database and upgrades in one transaction
  (Arch does not support partial upgrades).
- `UpgradeSecurity` applies security updates where supported, and
  `HasSecurityUpdates` reports whether any are available. Pacman returns
  `ErrUnsupported` for both security operations.
- `Upgrade` with an empty package list is a **no-op**, never a full upgrade —
  an accidentally-empty list must not upgrade the whole system; `UpgradeAll` is
  the explicit way to do that. `Autoremove` prunes no-longer-needed
  dependencies, and is a no-op on backends with no native equivalent.
<!-- docref: end -->

{% callout type="info" title="Reference" %}
The full method set and option fields are generated API docs on
[pkg.go.dev](https://pkg.go.dev/github.com/manchtools/cadestro/sdk/pkg).
This page is the task-oriented recipe; the reference lists the surface.
{% /callout %}

## Related

- Repositories (`sys/repo`) — configure the repositories these package managers
  install from.
- [Architecture](/concepts/architecture) — the Runner / Backend / Manager model.
- [Errors](/concepts/errors) — how failures are reported.
