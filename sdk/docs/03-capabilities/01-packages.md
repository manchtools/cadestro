---
title: Packages
label: Packages
description: Install, remove, and update software through native Linux package managers.
---

# Packages

`pkg.Manager` provides a uniform API for apt, dnf/dnf5, pacman, and zypper.
Cadestro's core uses it to install or remove a named package and to update the
whole system.

```go
backends := pkg.Detect()
if len(backends) == 0 {
    return errors.New("no supported package manager")
}
manager, err := pkg.New(backends[0], runner)
if err != nil {
    return err
}
_, err = manager.Install(ctx, pkg.InstallOptions{}, pkg.InstallSpec{Name: "vim"})
```

Package names and versions are validated before reaching the command line.
Mutations return the underlying command result and preserve their error cause.
