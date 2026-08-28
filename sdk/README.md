# Cadestro SDK

The public Go module behind Cadestro's minimal device-management core:

- native package management for apt, dnf/dnf5, pacman, and zypper;
- bounded, non-interactive command execution;
- the filesystem and systemd operations required by the agent;
- enrollment, certificate, and logging helpers.

Install it with:

```bash
go get github.com/manchtools/cadestro/sdk@latest
```

The module is pre-1.0 and may make breaking changes between minor releases.

Run the standalone gate with:

```bash
./scripts/verify.sh
```

The gate runs with `GOWORK=off` so it verifies the same dependency graph used
outside this repository.
