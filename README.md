# Cadestro

**Device management for Linux fleets that produces evidence, not just
outcomes.** One server binary, embedded SQLite, agents that connect outbound
over mTLS, and a transactional audit log that cannot be written around.

<!-- TERMINAL RECORDING GOES HERE — enrol a device and dispatch an action, ~30s. -->

---

## What it does

- **Declares desired state, and keeps asserting it.** Packages, files,
  directories, users, groups, services, repositories, SSH access, privilege
  policies, disk encryption, WiFi — each declared present or absent and
  re-converged on a schedule, not applied once and forgotten.
  → [capability reference](docs/capabilities.md)
- **Reaches machines you cannot reach.** Agents connect *out* to control and
  keep one mTLS stream open. Laptops behind NAT, on hotel WiFi, or asleep for a
  week are ordinary cases, not exceptions.
  → [enrollment](docs/enrollment.md)
- **Keeps working when the server is gone.** A disconnected agent keeps
  enforcing every policy it already holds, on schedule, indefinitely — and
  queues its results until it can report them.
  → [offline autonomy](docs/policy-model.md#6-offline-autonomy)
- **Records every change in the same transaction that makes it.** If the audit
  write fails, the change rolls back with it. There is no configuration in which
  a mutation happens un-audited.
  → [audit guarantees](docs/security-model.md#4-audit-guarantees)
- **Has no local accounts.** Identity is SCIM or OIDC from the directory you
  already run. No passwords, no TOTP — enforced by tests over the permission
  registry and the database schema, not by convention.
  → [identity](docs/security-model.md#2-identity)

## Quickstart

```bash
# 1. Install control (needs two DNS names and two filesystems — see the docs)
curl -fsSL https://github.com/manchtools/cadestro/releases/latest/download/install.sh -o install.sh
sudo bash install.sh

# 2. Get your one-time setup link, open it, configure your identity provider
cd /opt/cadestro && docker compose exec control cadestro bootstrap-admin

# 3. In the web UI, mint a registration token. Then, on a device:
curl -fsSL https://github.com/manchtools/cadestro/releases/latest/download/install.sh \
  | sudo bash -s -- --server https://control.example.com \
                    --token <TOKEN> --pin <CA_SHA256>
```

Full walkthrough: **[installation](docs/installation.md)** →
**[enrollment](docs/enrollment.md)**.

## Why "Cadestro"

The name comes from **cadastre** — the authoritative land register. Not a map,
not a list of who lives where, but *the* record: the one a court consults when
ownership is disputed, kept precisely so that it can be relied on later.

That is what the audit log is here, and it is the product's semantic root rather
than a feature beside it. Every mutation, every sensitive read, every rejected
authentication, and every background writer lands as an operation row plus its
effect rows, in the same database transaction as the change itself, on an
append-only hash chain enforced by database triggers, anchored off-host so that a
rewrite of the local chain is detectable. The effect schema has no free-form
field a secret could occupy. If the register cannot be written, the change does
not happen. Managing the fleet is what Cadestro does; being able to prove what
was done to it is what Cadestro is.

## Architecture

<!-- docref: begin src=server/internal/store/store.go#sqliteDSN:9dc3056b,server/internal/store/search.go#Store.Search:3244914e -->
One process, one file. The control plane is a single Go binary with embedded
SQLite in WAL mode at `synchronous=FULL`, and full-text search is SQLite FTS5 —
no database server, no cache, no queue, no search cluster.
<!-- docref: end -->

<!-- docref: begin src=contract/proto/cadestro/v1/agent.proto#AgentService:1027f6e5 -->
Devices connect outbound to a dedicated mutual-TLS listener and hold one
bidirectional stream that carries everything: handshake, sync, policy delivery,
results, secret operations, and terminal sessions. Nothing listens on the device.
<!-- docref: end -->

<!-- docref: begin src=server/deploy/compose.yml#@deployment-services:ca913fd3 -->
The reference deployment is three containers behind Traefik — ingress, control,
and the web UI served same-origin beside it — with agents on a separate,
internal-only network.
<!-- docref: end -->

## Status and scope

**Pre-1.0. Reinstall, not upgrade.** Within a version, updating is a container
image pull. Across a schema version there is no migration path — the server
refuses a database it does not recognise rather than guessing.
→ [upgrades](docs/upgrade.md)

<!-- docref: begin src=.github/workflows/agent-integration.yml#@distro-matrix:8502f31d,.github/workflows/sdk.yml#@sdk-distro-matrix:3fc7efef -->
**Tested on**, in CI, against real system state in containers: the agent's
executor on Debian, Fedora, openSUSE, and Arch; the system capability layer on
Fedora, AlmaLinux, Arch, and openSUSE in addition to its Debian base lanes.
Other distributions may work; these are the ones a merge cannot break silently.
<!-- docref: end -->

<!-- docref: begin src=server/internal/store/sqlite_scale_test.go#TestSQLiteScale_MixedWorkloadAtTenThousandAgents:cdb39986 -->
**Scale target: one control instance for 10,000 connected agents.** There is a
gate for it — a mixed dispatch, receipt, result, heartbeat, terminal, and search
workload at that agent count with latency assertions. It is **operator-run, not
CI-run**: it skips unless explicitly enabled, so treat the number as a target
this repository can demonstrate on request, not as something every merge proves.
High availability and multi-region are out of scope.
<!-- docref: end -->

## How this compares

Configuration management tools — Ansible, Salt, Puppet — largely assume the
control plane can reach the machine, on a schedule the control plane chooses.
That assumption holds in a datacentre and breaks for laptops.

Cadestro inverts it: the device connects out and stays connected, keeps
enforcing policy when it cannot, and records what happened as transactional
evidence rather than run output. If your fleet is servers you can SSH to and you
want a general-purpose automation language, use one of those tools — they are
better at it. If your fleet is end-user Linux machines you cannot reach on
demand, and you need to be able to prove later what was changed on them, that is
the gap this fills.

## Licensing

| Module | Licence | |
|---|---|---|
| [`contract/`](contract/) | MIT | The wire contract — implementing the protocol must impose no obligations |
| [`sdk/`](sdk/) | MIT | Reusable Linux system capabilities, meant to be embedded elsewhere |
| [`agent/`](agent/) | GPL-3.0 | Ships to managed devices |
| [`server/`](server/) | AGPL-3.0 | The control plane; network use counts as distribution |
| [`web/`](web/) | AGPL-3.0 | The administration UI |

Each module's `LICENSE` file governs that module. See
[LICENSING.md](LICENSING.md) for why the split is what it is and how the
dependency direction is enforced.

**Commercial licensing and support:** contact the maintainers through
[the repository](https://github.com/manchtools/cadestro).

## Documentation

- **[Policy model](docs/policy-model.md)** — start here.
- [Installation](docs/installation.md) · [Enrollment](docs/enrollment.md) ·
  [Backup and restore](docs/backup-restore.md) · [Upgrades](docs/upgrade.md)
- [Capability reference](docs/capabilities.md) ·
  [Security model](docs/security-model.md)

## Repository layout

| Path | Contents |
|---|---|
| `contract/` | Protocol buffers (the single source of truth) plus the generated Go and TypeScript clients |
| `sdk/` | Reusable Linux system capabilities |
| `agent/` | The device agent, `cadestrod`, and its installer |
| `server/` | The control plane, `cadestro`, with the reference deployment under `server/deploy/` |
| `web/` | The administration UI |
| `docs/` | Product and operator documentation |
| `.github/workflows/` | CI for every module — GitHub honours workflows only at the repository root, so there is no module-local CI |
| `scripts/` | The repository-level gates (`verify.sh`, `verify-all.sh`) |

Contributing: [CONTRIBUTING.md](CONTRIBUTING.md). Security reports:
[SECURITY.md](SECURITY.md). How this repository was assembled:
[PROVENANCE.md](PROVENANCE.md).
