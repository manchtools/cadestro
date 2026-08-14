# Cadestro

Device management for Linux fleets — one server binary, embedded SQLite, mTLS
agents, and a transactional audit log.

**This repository is being assembled.** Cadestro consolidates its predecessor's
component repositories into one codebase under a new name; the sources were
seeded module by module and the documentation is being rewritten from the code
as part of the move. [PROVENANCE.md](PROVENANCE.md) records exactly what was
seeded, from where, and what the squash discarded.

## Layout

| Module | License | Contents |
|---|---|---|
| `contract/` | MIT | Protocol buffers (single source of truth), generated Go and TypeScript clients |
| `sdk/` | MIT | Reusable Linux system capabilities |
| `agent/` | GPL-3.0 | The device agent, `cadestrod` |
| `server/` | AGPL-3.0 | The control plane, `cadestro`, and the reference deployment under `server/deploy/` |
| `web/` | AGPL-3.0 | The administration UI |

Those five are the modules. `.github/workflows/` holds the CI for all of them —
GitHub honours workflows only at the repository root, so there is no
module-local CI — and `scripts/` holds the repository gates.

See [LICENSING.md](LICENSING.md) for how the per-module licensing works and
[CONTRIBUTING.md](CONTRIBUTING.md) for the build, test, and verification
commands.
