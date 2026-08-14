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
| `crypto/` | Enrollment CSR, at-rest AEAD, and transport-field sealing helpers |
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

## The contract dependency

This module is otherwise free of the generated protobuf types, and the
leaf-purity guard in `archtest` keeps it that way. `sys/osquery` is the single
recorded exception: its `Querier` API is expressed in the contract's `OSQuery`
messages, so it imports `contract/gen`. The guard fails both when a second
package starts importing the contract and when that exception goes stale.

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
env GOWORK=off go test ./...
docref check
```

See [`CONTRIBUTING.md`](CONTRIBUTING.md) for contribution mechanics and
[`docs/04-contributing/01-release-coordination.md`](docs/04-contributing/01-release-coordination.md)
for how the modules resolve each other.
