# Security policy

## Reporting a vulnerability

Report vulnerabilities privately to the repository maintainers. Do not open a
public issue containing exploit details, credentials, or affected deployment
data.

## Architecture authority

The sole system security design is
`../DESIGN_2026_07_31/00_TARGET_DESIGN.md`. This file describes the
repository's security contract without creating a competing architecture.

## Trust boundaries

- Traefik is the only internet-facing server component.
- The browser authenticates to control with an OIDC-derived session.
- Agents authenticate directly to control with device mTLS certificates.
- The embedded SQLite database is trusted for availability and persistence, but
  a database copy must not reveal plaintext protected secrets.
- The control host and its CA/key material are trusted. A hostile host
  administrator is outside the application threat boundary.

## Required guarantees

### Identity and authorization

Human login is OIDC only. SCIM owns managed provisioning and erasure; providers
without SCIM may opt into OIDC JIT provisioning, whose subjects can be erased
only through the provenance-gated local JIT-erasure path. MFA belongs to the
identity provider. The bootstrap-admin token is single-use, short-lived, and
cannot act as an ordinary `:self` user.

Every trust-boundary input is validated before authentication and
authorization. Object-scoped non-owner access returns NotFound. Privilege-
widening permissions remain global-only.

### Device transport

The device generates its own Ed25519 key and CSR. Its private key never leaves
the device. Control terminates mTLS, derives device identity from the
certificate, checks the device's active serial during handshake and privileged
frames, and rejects a live stream as soon as another certificate is promoted.

Ordinary application frames are not separately signed. There is no untrusted
relay or offline verifier between agent and control.

### Secrets

Classified protobuf fields carry raw bytes only inside the authenticated mTLS
device stream. The peer certificate supplies the device identity; there is no
second application envelope or caller-supplied device binding.

At rest, secret values use AES-256-GCM with resource-context AAD and distinct
domain tags. Values are decrypted only on a fresh outbound copy or at an
explicit audited reveal sink.

Logging is metadata-only. Secrets must not enter debug formatting, logs,
errors, traces, audit payloads, or support bundles.

### State and audit

Application state is ordinary CRUD. The audit log is append-only evidence, not
authoritative state. Ordinary mutations and their initial operation/effect rows
commit in one transaction. Coalesced heartbeat liveness is the sole named
unaudited telemetry writer. Shared boundaries and exact-set tests enforce
coverage for RPCs, sensitive reads, rejected authentication, SCIM, enrollment,
jobs, and background writers.

Audit streams are hash-chained and periodically anchored in the configured
archive. Retention archives and verifies a prefix before deletion.

<!-- docref: begin src=cmd/cadestro/config.go#validateArchiveIsolation:69e8ec75,cmd/cadestro/devauth_stub.go#archiveIsolationRelaxed:8de98d35 -->
Filesystem separation is enforced. Control compares the filesystem holding
the archive path with the one holding the database and refuses to start when
they match. Off-host replication remains an operator requirement. No
configuration variable relaxes the filesystem check; the only build that
tolerates a shared filesystem is the same one that mints administrator
sessions without an identity provider.
<!-- docref: end -->

### Dispatch

Control commits a complete delivery before send. The agent durably records
receipt before acknowledgement. Stable delivery IDs, connection epochs,
idempotent result ingestion, and an explicit INDETERMINATE outcome prevent
silent replay of non-idempotent effects.

## Deployment requirements

- Do not expose control directly to the internet.
- Do not mount the Docker socket into Traefik.
- Restrict the PROXY-protocol listener to the isolated Traefik network.
- Protect CA, JWT, sealing, database, and at-rest encryption keys with strict
  filesystem or deployment-secret permissions.
<!-- docref: begin src=cmd/cadestro/config.go#validateArchiveIsolation:69e8ec75,cmd/cadestro/devauth_stub.go#archiveIsolationRelaxed:8de98d35,deploy/backup.sh#@sqlite-backup:99bc90ed,cmd/cadestro/backup_status.go#runBackupStatus:41ed4e6c -->
- Give `CADESTRO_BACKUP_PATH` its own filesystem, separate from the one
  holding the database: it carries the audit chain's tamper-evidence, and
  control refuses to start when the two share a mount. `deploy/setup.sh` makes
  the same check before it renders a configuration, so a deployment installed
  from this tree either has that storage or does not install.
- Run `deploy/backup.sh` from a host timer and replicate the
  `CADESTRO_BACKUP_PATH` directory off-host: it contains verified bounded
  SQLite backups, audit anchors, and archived prefixes. Back up artifacts too,
  and monitor `cadestro backup-status`.
<!-- docref: end -->

Gateway, Valkey, Asynq, external indexing, CRL distribution, local
password/TOTP, and application-frame signing are not compensating controls and
must not be reintroduced.
