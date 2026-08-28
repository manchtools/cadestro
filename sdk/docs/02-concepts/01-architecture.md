---
title: Architecture
label: Architecture
description: Explicit runners and package-manager backends.
---

# Architecture

Cadestro injects an `exec.Runner` into host-management code. The caller
chooses direct execution, sudo, or doas; package management separately chooses
apt, dnf/dnf5, pacman, or zypper.

```go
runner, err := exec.NewRunner(exec.Direct)
if err != nil {
    return err
}
manager, err := pkg.New(pkg.Apt, runner)
```

There is no process-global backend. Package mutations return an `exec.Result`
containing exit status, stdout, and stderr.
