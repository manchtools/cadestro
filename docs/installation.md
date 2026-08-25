# Installation

Standing up a Cadestro control plane. The reader this page assumes is a Linux
admin with Docker Compose, a domain, and a machine with a public address.

Plan for **two DNS names**. They are architectural, not a preference, and are
checked before anything starts.

---

## What you need first

The installer checks for `curl`, `tar`, `docker`, and `openssl`, and that the
Docker Compose plugin responds. It does not check for root, port availability,
free disk, or a minimum Docker version — so satisfy those yourself.

**Two hostnames**, both resolving to this machine:

<!-- docref: begin src=server/deploy/install.sh#check_agent_domain:76d1a1bd,server/deploy/install.sh#print_dns_reminder:44a50643 -->
- a **browser/API domain** — where admins and the web UI live;
- an **agent domain** — where devices connect. The installer refuses an agent
  domain equal to the browser one. They are separate because agent traffic is
  passed through to control without TLS termination while browser traffic is
  terminated at the proxy; one name cannot be routed both ways.
<!-- docref: end -->

---

## Running the installer

```bash
curl -fsSL https://github.com/manchtools/cadestro/releases/latest/download/cadestro-install.sh -o install.sh
sudo bash install.sh
```

<!-- docref: begin src=server/deploy/install.sh#ask:7471c3b3 -->
It prompts only on a terminal, and only for values not already set in the
environment. An environment value always wins, so the same script drives both an
interactive install and an unattended one — set the variables and nothing is
asked.
<!-- docref: end -->

It asks, in order:

| Prompt | Configures | Notes |
|---|---|---|
| Browser/API domain | `CONTROL_DOMAIN` | must be a hostname; the documentation placeholder is rejected |
| Agent domain | `AGENT_DOMAIN` | must differ from the browser domain |
| Email for expiry notices | `ACME_EMAIL` | goes to Let's Encrypt |
| Release to install | `RELEASE_TAG` | release installers default to their stamped release; source copies require one |
| Certificate challenge | `ACME_CHALLENGE` | `http01` (default) or `dns01` |
| DNS provider code | `ACME_DNS_PROVIDER` | only asked for `dns01` |

<!-- docref: begin src=server/deploy/install.sh#check_control_domain:213169ce,server/deploy/install.sh#check_acme_email:77a9e4af,server/deploy/install.sh#check_release_tag:dfdd9ce2 -->
Each answer is validated and re-asked until it is valid. The validators reject
the literal placeholder values used in the documentation, so a copy-pasted
example domain or example email cannot become a live configuration by accident.
The published installer carries its exact release tag, so it never silently
installs whatever is newest. A source copy requires `RELEASE_TAG` explicitly.
<!-- docref: end -->

**No prompt ever asks for a secret.** Credentials for a DNS challenge are placed
in a file, not typed into a prompt that could reach a shell history or a
terminal log.

The installer then copies the deployment tree into `/opt/cadestro` (override
with `INSTALL_DIR`), writes `.env` at mode `0600`, runs `setup.sh`, pulls the
images, and brings the stack up.

> **Worth knowing:** the installer verifies the signed `SHA256SUMS` manifest
> before unpacking the release deployment archive.

---

## Data and backups

Control stores the database under `data/control` and ordinary verified backup
artifacts under `data/backups`. The backup script uses SQLite's online backup API;
see [Backup and restore](backup-restore.md) for scheduling and recovery.

---

## What setup generates

<!-- docref: begin src=server/deploy/setup.sh#main:19109030 -->
`setup.sh` is the enforcing half of the installation. It never downloads, never
prompts, and never touches Docker: it validates the environment, creates the
directory tree with restrictive permissions, writes the ACME overlay, generates
the key material, renders the service
configuration, and verifies the resulting permissions.
<!-- docref: end -->

<!-- docref: begin src=server/deploy/setup.sh#load_environment:57a835bd -->
It parses `.env` as plain key/value assignments rather than sourcing it. A
configuration file is data; sourcing it would make it code, and a stray
backtick in a domain name would execute.
<!-- docref: end -->

<!-- docref: begin src=server/deploy/setup.sh#@generated-material:c44d1838 -->
Generated once and then retained: the internal CA, the control-plane server
certificate, the at-rest encryption key, and the session signing key.
<!-- docref: end -->

<!-- docref: begin src=server/deploy/setup.sh#ensure_secret_files:7282f65d -->
Existing material is never regenerated — which is what lets you pre-provision a
CA from your own PKI. A half-present pair fails rather than being completed with
a fresh key that would not match its certificate.
<!-- docref: end -->

<!-- docref: begin src=server/deploy/setup.sh#ensure_certificates:a840a2ee -->
Certificate handling keeps the active internal CA and control certificate
together. Existing material is reused only when it verifies against that CA;
the deployment does not maintain a rotating trust bundle.
<!-- docref: end -->

<!-- docref: begin src=server/deploy/setup.sh#certificate_covers_host:0feefa5b -->
One detail worth knowing if you ever debug this: the hostname check reads
OpenSSL's textual output rather than its exit status. On the OpenSSL 3.0 that
ships with current Debian and Ubuntu, the exit status is zero even when the
hostname does not match — so trusting it would have silently accepted a
certificate for the wrong host.
<!-- docref: end -->

<!-- docref: begin src=server/deploy/setup.sh#validate_permissions:6ccb550f -->
Finally it verifies the result rather than assuming it: the data directories
must be writable, and every key, secret, and rendered environment file must have
no group or world bits.
<!-- docref: end -->

---

## The stack

<!-- docref: begin src=server/deploy/compose.yml#@deployment-services:9a1eca5a -->
Three services, and **only Traefik publishes ports** — 80 and 443. Control and
the web UI have no published ports at all; they are reachable only across the
compose networks.

- **traefik** — the ingress, pinned to an exact version rather than a moving
  tag.
- **control** — the control plane, holding the certificates and secrets as
  read-only mounts and the database, artifacts, and backups as read-write ones.
- **web** — the administration UI. Static build output: no volumes, no secrets,
  and no access to the agent network.

Agents reach control over a second, internal-only network, so a compromise of
the browser-facing path does not put an attacker on the device network.
<!-- docref: end -->

### Routing

<!-- docref: begin src=server/deploy/traefik/dynamic/routes.yml#@public-backend-tls:4a18996e -->
The browser domain serves both the API and the UI **same-origin**, split by path
priority rather than by hostname or subpath: the control plane's RPC, SCIM,
terminal, and health paths take precedence, and everything else falls through to
the web UI at the root. Both priorities are stated explicitly so the split does
not silently depend on how Traefik ranks rule lengths.

Traefik re-originates TLS to control — it does not forward plaintext to the
backend — verifying it against the active deployment CA with TLS 1.3 pinned as
both the minimum and the maximum version.
<!-- docref: end -->

<!-- docref: begin src=server/deploy/traefik/dynamic/routes.yml#@agent-route:2b16b515 -->
Agent traffic is a TCP router with **TLS passthrough**: Traefik does not
terminate it, so the mutual-TLS session runs end to end between the device and
control. It prepends a PROXY protocol v2 header so control still learns the real
client address.
<!-- docref: end -->

<!-- docref: begin src=server/deploy/traefik/traefik.yml#@safe-access-log:18c7080a -->
The access log drops the request path, the request line, and all headers by
default. An ingress log is a standing risk of recording a token someone put in a
URL; this one is configured not to be able to.
<!-- docref: end -->

### TLS

<!-- docref: begin src=server/deploy/setup.sh#ensure_traefik_acme_config:112fe600 -->
Two ACME challenge types are supported. `http01` needs port 80 reachable from
the internet and nothing else. `dns01` needs a provider code and API credentials,
and works when port 80 is not reachable — or when you want a certificate before
the name is publicly routable.

The DNS credentials go in their own file that Traefik reads directly, so they
never become a Cadestro configuration variable and never pass through the
installer. Setup checks only that the file exists, is non-empty, and is not
group- or world-readable — and **refuses** rather than repairing a permissive
one. The variable inside is whatever your provider expects; the provider code
comes from the ACME library's provider list.
<!-- docref: end -->

Both hostnames must resolve to this machine. The browser domain gets a Let's
Encrypt certificate; the agent domain is a distinct name for the passthrough
route, where control presents its own internally issued certificate.

---

## First login

There is no default account and no default password — see
[the security model](security-model.md#2-identity). The first login is a
single-use token minted on the host.

```bash
cd /opt/cadestro
docker compose exec control cadestro bootstrap-admin
```

<!-- docref: begin src=server/cmd/cadestro/bootstrap_admin.go#writeBootstrapAdminOutput:665c9c92 -->
It prints a setup URL and the exact time it expires. The token travels in the
URL **fragment**, which browsers do not send to the server — so it cannot land
in the ingress access log or a server-side request log on its way to the page
that consumes it.
<!-- docref: end -->

Open the URL. The setup page reads the token out of the fragment, holds it in
memory, and spends it configuring your first identity provider. From then on you
log in through that provider.

<!-- docref: begin src=server/cmd/cadestro/main.go#parseCommand:dfecd82c -->
The control binary accepts exactly two subcommands — `bootstrap-admin` and
`backup-status`. Anything else exits with a usage error rather than being
interpreted.
<!-- docref: end -->

If the token expires — it is valid for 15 minutes — just run the command again.
Issuing a new one retires the old.

---

## Configuration reference

<!-- docref: begin src=server/cmd/cadestro/config.go#optionPrefix:e975db97,server/cmd/cadestro/config.go#readEnvironment:88fc4d61 -->
Control is configured entirely through `CADESTRO_`-prefixed environment
variables, rendered into `config/control.env` by setup. The recognised set is
derived from the configuration structure itself rather than maintained as a
second list, and **an unrecognised `CADESTRO_` variable is a startup failure**,
not a warning. A typo in a security-relevant setting stops the server instead of
silently leaving the default in place. Error messages name variables, never
their values.
<!-- docref: end -->

<!-- docref: begin src=server/cmd/cadestro/config.go#validateConfig:fde0feb4,server/cmd/cadestro/config.go#validateWritableDirectory:0881f863 -->
Validation is thorough and happens before anything opens a socket: the two
listen addresses must be present and distinct; the proxy sources must be valid
addresses or CIDRs; the public base URL must be absolute HTTPS with no
credentials, query, or fragment; the terminal URL must be a `wss://` URL; the
database path must be absolute; the session key must be exactly an Ed25519
private key; and the artifact and backup directories are proven writable by
actually creating and removing a file, not by inspecting mode bits.
<!-- docref: end -->

<!-- docref: begin src=server/internal/store/store.go#sqliteDSN:9dc3056b,server/internal/store/store.go#prepareSQLiteFile:736f513d -->
The datastore is embedded SQLite in WAL mode with `synchronous=FULL`, foreign
keys enforced, and a five-second busy timeout. The database file is chmod'ed to
owner-only on every open, so a permissive mode cannot survive a restart.
<!-- docref: end -->

---

## Verifying it works

```bash
docker compose ps                    # all three healthy
curl -fsS https://control.example.com/ready
```

<!-- docref: begin src=server/cmd/cadestro/readiness.go#checkReadiness:654959a2 -->
Readiness checks that the database answers and that the active-certificate
lookup works, and
that the artifact path is writable. When backup posture is configured, a
missing, invalid, failed, or stale backup also makes readiness fail. An
explicitly disabled or unconfigured backup policy is skipped. Backup posture
is also reported separately (see [backup and restore](backup-restore.md)).
<!-- docref: end -->

Then enroll your first device — see [enrollment](enrollment.md).

---

## Where to go next

- [Enrollment](enrollment.md) — getting devices connected.
- [Backup and restore](backup-restore.md) — set this up before you have
  anything to lose.
- [Upgrades](upgrade.md) — how updates work before 1.0.
- [Security model](security-model.md) — what the above is protecting.
