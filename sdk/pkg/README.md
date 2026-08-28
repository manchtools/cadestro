# Native package management

Package `pkg` provides one interface for apt, DNF/DNF5, pacman, and zypper.
It detects an installed backend or accepts an explicit backend, executes every
mutation non-interactively, and returns typed errors and captured command
results.

```go
r, err := exec.NewRunner(exec.Direct)
if err != nil {
    return err
}

m, err := pkg.New(pkg.Auto, r)
if err != nil {
    return err
}

_, err = m.Install(ctx, pkg.InstallOptions{}, "curl")
```

The core mutation surface is package install/remove and full-system update.
Read operations remain available for status and diagnostics. Package names,
versions, search terms, local package paths, and GPG key references are
validated before execution.

Use `exec.Direct` when the caller already runs as root. `exec.Sudo` and
`exec.Doas` use non-interactive escalation and fail closed if authorization
would require a password.
