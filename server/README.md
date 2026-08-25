# Cadestro

Self-hosted device management for Linux fleets — one binary, embedded SQLite,
mTLS agents, and a transactional audit log.

Cadestro is for teams running tens to thousands of Linux machines —
workstations, servers, kiosks — who need enrollment, desired-state policy,
live device control, and audit evidence without operating a database cluster
to get them.

## What it does

- **Enrolls devices over their own outbound connection.** The agent generates
  an Ed25519 key on the device, sends a CSR with a single-use token, pins the
  control CA, and keeps one outbound mTLS stream open. No inbound ports on
  endpoints, no SSH reachability requirement.
- **Applies desired state on a schedule, online or offline.** Actions declare
  `PRESENT`/`ABSENT` for packages (apt, dnf/dnf5, pacman, zypper — plus flatpak,
  deb, rpm, AppImage), services, files, users and groups, SSH and sshd policy,
  disk encryption, Wi-Fi, and more. Agents store their manifests durably and
  re-apply them on cron or drift intervals even without a server connection,
  honoring per-device maintenance windows.
- **Applies assigned policy on synchronization.** Devices receive their
  compiled assignment snapshot, retain scheduled work locally, and reconcile
  it on drift intervals even while control is unavailable.
- **Produces audit evidence by construction.** Every mutation commits in the
  same transaction as its audit operation and effect rows; if audit
  persistence fails, the state change rolls back. Sensitive reads are their
  own audited operations, and secret values never enter logs or audit
  payloads.
<!-- docref: begin src=internal/scim/users_write.go#Handler.provisionSubject:90234164,internal/idp/linker.go#Linker.createUser:7858b2ad,internal/identity/users.go#Handlers.EraseJITUser:993bfcf8 -->
- **Uses enterprise identity from day one.** Human accounts come from SCIM
  lifecycle management or per-provider OIDC just-in-time creation — there is
  no manual user creation and no local passwords. JIT-created subjects have an
  explicit, provenance-gated erasure RPC. First-admin bootstrap is a one-time,
  host-authorized token.
<!-- docref: end -->

Compliance is detection-only by design: policies run detection scripts that
yield a per-device status (compliant, non-compliant, in grace period) —
evidence, not silent remediation.

## Quickstart

You need a Linux host with Docker Compose, two DNS names pointing at it (one
for the browser/API, one for agent mTLS), an email for Let's Encrypt, and an
OIDC provider for operator login. The installer is interactive and asks for
everything it needs; it never prompts for secrets.

```bash
curl -fsSL https://raw.githubusercontent.com/manchtools/cadestro/main/deploy/install.sh -o install.sh
chmod +x install.sh && sudo ./install.sh
```

Then create a single-use administrator setup URL, configure your identity
provider through it, and enroll a device:

```bash
# on the control host: host-authorized, single-use setup URL
docker compose exec control cadestro bootstrap-admin

# in the browser at that URL: configure OIDC and SCIM, then mint an
# enrollment token

# on the device, with the installer from the agent release assets
sudo bash install.sh -s https://agents.example.com -t <token> -p <ca-fingerprint>
```

The bearer token sits in the URL fragment, which browsers do not send to
control or to Traefik access logs. There is no local password or TOTP
administrator. The device agent ships from this repository's `agent/` module
with signed releases the installer verifies before anything lands on disk.
Full walkthrough: [deploy/QUICKSTART.md](deploy/QUICKSTART.md).

## Architecture

One control process owns the API, the dedicated agent mTLS listener, identity,
authorization, device control, search, and audit, with all state in an embedded
SQLite database (WAL mode, `synchronous=FULL`, FTS5 search). Agents connect
outbound; control never dials a device. There is no external database, queue,
or cache to operate.

## Status and scope

Pre-1.0 release candidates. Goose is embedded in the control plane and applies
ordered SQLite migrations automatically at startup. The current pre-1.0
history is a single squashed baseline; once a schema is released, later
changes are new migrations with tested `Up` and `Down` sections.

**CI-tested**, with real package managers and system services in the test
matrix: Debian bookworm, Fedora, Arch, openSUSE (Leap and Tumbleweed), Ubuntu
(apt path), AlmaLinux 9 (library level). Other systemd-based distributions
generally work but are not exercised in CI; non-systemd systems are not
supported.

**Scale:** designed for up to 10,000 normally connected agents on a single
instance. A checked-in scale gate exercises that state volume with hard
latency assertions; it is operator-run rather than continuous CI.

**Deliberately out of scope for version one:** high availability,
multi-region, local passwords/TOTP/WebAuthn, and email notifications
(alerting is a generic HTTPS webhook).

## Where it fits

Configuration management (Ansible, Puppet, Salt) pushes changes to reachable
machines and records that a run happened. Cadestro enrolls devices that
connect outward, keeps desired state applied while they are offline, and
records every change as transactional audit evidence — which matters for
remote fleets and for anyone who has to hand an auditor enrollment records
and continuous policy state rather than playbook logs. Compared to existing
MDM platforms, Linux is the first platform here, not the last checkbox.

## License

The server is [AGPL-3.0](LICENSE). The device agent is GPL-3.0, and the
protobuf contract, SDK, and operator CLI are MIT — you can build your own
client against the published contract.

Contributions: see [CONTRIBUTING.md](CONTRIBUTING.md).
