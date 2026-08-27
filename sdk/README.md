# Cadestro SDK

Reusable Linux system capabilities: package managers, users, services,
filesystems, disk encryption, networking, antivirus, CA trust, and more, behind
one consistent, dependency-injected API — plus the crypto helpers the agent and
control server share.

MIT, and intended to be embedded in other people's tools. See
[`../LICENSING.md`](../LICENSING.md).

## Repository layout

| Path | Purpose |
|------|---------|
| `sys/` | Device capability implementations |
| `pkg/` | Package-manager capabilities |
| `crypto/` | Enrollment CSR, certificate, and at-rest AEAD helpers |
| `cryptotest/` | Shared X.509 fixtures for the capability and agent test suites |
| `logging/` | Metadata-only logging helpers |
| `archtest/` | Architectural fitness functions over this module |
| `adversary/` | Cross-capability adversarial suites |
| `docs/` | Capability and contributor documentation |

## Design

Every capability takes an injected `exec.Runner` and, where more than one
implementation is genuinely supported, an explicit `Backend`. There is no
process-global backend selector and no ambient runner; `archtest` fails the
build when either reappears. Shipped implementations are concrete — systemd for
service actions, LUKS for disk encryption — and the public API does not expose
selectors for implementations that do not exist.

## Leaf purity

This module is proto-free and imports nothing else in this repository; the
leaf-purity guard in `archtest` fails the suite the moment any package —
test files included — grows an in-repo import edge.

## Verify

Run the canonical standalone-module gate:

```bash
./scripts/verify.sh
```

It checks formatting, build, vet, static analysis, Go tests, and docref.
`GOWORK=off` is intentional so the result matches how a consumer outside this
repository resolves the module.

Useful focused commands:

```bash
env GOWORK=off go test -p 1 ./...
docref check
```

See [`CONTRIBUTING.md`](CONTRIBUTING.md) for contribution mechanics and
[`docs/04-contributing/01-release-coordination.md`](docs/04-contributing/01-release-coordination.md)
for how the modules resolve each other.
