---
title: Backends
label: Backends
description: Explicit native package-manager selection.
---

# Backends

`pkg.Detect` lists installed native package managers in priority order.
`pkg.New` requires the caller to choose one of apt, dnf/dnf5, pacman, or
zypper and rejects the zero value.

Detection reports availability; it never changes a manager after construction.
