# Cadestro server quickstart

<!-- docref: begin src=deploy/compose.yml#@deployment-services:c3bfad13 -->
The stack has exactly three services: Traefik, one control process with an
embedded SQLite database, and the administration UI. Compose gives control no
arguments and passes it the rendered `config/control.env` as the container's
environment file, and passes Traefik `config/traefik-acme.env` and
`config/traefik-dns.env`, and the UI `config/web.env`, the same way. The UI is
served on `CONTROL_DOMAIN` alongside the API rather than on a hostname of its
own, so the browser's API origin is the page's own origin and a fresh install
needs no client configuration. Both images are released from one repository
under one tag, which is why they share `IMAGE_TAG`.
The authoritative system design is
`../../DESIGN_2026_07_31/00_TARGET_DESIGN.md`.
<!-- docref: end -->

## Prepare

Copy `.env.example` to `.env`, set the three required public values
(`CONTROL_DOMAIN`, `AGENT_DOMAIN`, and `ACME_EMAIL`), then run `./setup.sh`. `.env` carries only those values,
the optional ACME challenge selection below, and `IMAGE_TAG`; it is Compose's
own environment file, not control's configuration.

Run `./setup.sh` to render the fresh deployment configuration. The audit log is stored transactionally in SQLite alongside the state it describes.

### Certificates without a reachable port 80

Let's Encrypt proves you own `CONTROL_DOMAIN` and `AGENT_DOMAIN` before it
issues anything. By default it does so over HTTP, which requires this host to
be reachable from the internet on port 80. Behind CGNAT, on a residential line
where port 80 is blocked, or on a private network, it is not — set the ACME
challenge to DNS instead and Traefik proves ownership by writing a record into
your DNS zone:

```
ACME_CHALLENGE=dns01
ACME_DNS_PROVIDER=hetzner
```

`ACME_DNS_PROVIDER` is a [lego provider
code](https://go-acme.github.io/lego/dns/). The zone has to be served by that
provider, not merely registered there: Hetzner DNS answers for the domain only
once its nameservers are the ones delegated to it. Write that provider's
credentials into `config/traefik-dns.env`, one `KEY=VALUE` per line — for
Hetzner DNS, `HETZNER_API_TOKEN` with a Cloud Console API token (the
`HETZNER_API_KEY` variable selects the legacy DNS API that Hetzner shut down
in May 2026, which fails with an HTML-instead-of-JSON unmarshal error):

```bash
mkdir -p config && install -m 600 /dev/null config/traefik-dns.env
printf 'HETZNER_API_TOKEN=%s\n' "$token" >> config/traefik-dns.env
```

Traefik reads that file itself. `setup.sh` never copies or prints its contents,
and refuses a `dns01` run when the file is missing, empty, or readable by other
accounts — before it generates any key material, so a corrected run starts from
nothing. It renders the challenge selection into `config/traefik-acme.env`,
pins the propagation check to public resolvers — a split-horizon resolver at
home answers from the internal view of the zone, where the challenge record
does not exist, and the order would never complete — and waits 60 seconds
before that check, because the certificate authority validates from several
vantage points and a record one resolver already sees can still be missing at
the DNS operator's other anycast nodes.

Set `ACME_CHALLENGE` and `ACME_DNS_PROVIDER` in `install.sh`'s environment and
it writes them into the `.env` it generates, but the credentials file can only
be written after it has unpacked the tree. Write the file into the install directory it created, then run
`./setup.sh && ./deploy.sh` there rather than `install.sh` again.

Leave both unset for the default HTTP challenge; port 80 also carries the
redirect to HTTPS either way, so keep it published.

Control is configured entirely by `CADESTRO_`-prefixed environment
variables and reads no configuration file. `setup.sh` renders every one of them
into `config/control.env`, and that file is where ordinary settings such as the
log level or the log settings are edited. `setup.sh` re-renders it on
every run, including through `./deploy.sh`, so re-apply local edits afterwards.

<!-- docref: begin src=deploy/setup.sh#@generated-material:6df12966 -->
`setup.sh` creates the internal Ed25519 CA, the control certificate, the
encryption and session keys, `config/control.env` with the SQLite `CADESTRO_DATABASE_PATH`, and
`config/web.env` with the `PUBLIC_CONTROL_URL` the UI calls — the same origin
control publishes its setup URL on, taken from `CONTROL_DOMAIN`. It validates
the chosen ACME challenge before generating key material — an
unknown `ACME_CHALLENGE`, `dns01` without an `ACME_DNS_PROVIDER`, or
`config/traefik-dns.env` missing, empty, or readable by other accounts. It then
renders the challenge into `config/traefik-acme.env`, and creates an empty
`config/traefik-dns.env` for an `http01` deployment so Compose always has the
file it references. Existing complete keypairs are retained, which permits a
pre-provisioned CA. Partial or unusable material fails closed. Generated secret
files and every file under `config/` are mode 0600, verified before the script
reports success, and no secret value is ever printed.
<!-- docref: end -->

<!-- docref: begin src=deploy/traefik/dynamic/routes.yml#@agent-route:2b16b515,cmd/cadestro/httpserver.go#serveAgent:0543d07f,cmd/cadestro/httpserver.go#buildAgentServer:ccd04d34,internal/agentstream/identity.go#MTLSMiddleware:306e83b6 -->
The public and agent hostnames must differ. Traefik terminates browser/API TLS
for `CONTROL_DOMAIN`. For `AGENT_DOMAIN`, it passes TLS through and adds PROXY
protocol v2 on an isolated network; control itself authenticates the device
certificate and checks its active serial.
<!-- docref: end -->

<!-- docref: begin src=deploy/traefik/dynamic/routes.yml#@public-backend-tls:da534a3f,deploy/traefik/traefik.yml#@safe-access-log:e383937a -->
Traefik also authenticates control's internal TLS certificate against the
deployment CA, so browser/API traffic stays encrypted after public TLS
termination. Control keeps the paths it serves — the `cadestro.v1.ControlService`
procedures, `/scim`, `/terminal`, `/health`, and `/ready` — at a higher router
priority, and everything else on that hostname is the UI, including the `/setup`
page the bootstrap URL points at. The hop to the UI container is plain HTTP on
the internal bridge: it serves build output, holds no secret, and that hop never
leaves the Compose network. Its JSON access log omits the URI-bearing `RequestPath` and
`RequestLine` fields; method, host, status, timing, router, service, and client
metadata remain available without recording query-string credentials.
<!-- docref: end -->

## Start

Run `docker compose up -d --wait`, then inspect the result with
`docker compose ps`. The administration UI answers at `https://$CONTROL_DOMAIN`
and needs no configuration of its own — the setup URL below opens in it.

<!-- docref: begin src=cmd/cadestro/bootstrap_admin.go#runBootstrapAdmin:fd19e1f2,internal/identity/bootstrap.go#Bootstrapper.setupURL:417b204e -->
Create a host-authorized, single-use administrator setup URL:

Run `docker compose exec control cadestro bootstrap-admin`.

The bearer token is placed in the URL fragment, which browsers do not send to
control or Traefik access logs.
<!-- docref: end -->

Use that session to configure OIDC and SCIM. There is no local password or TOTP
administrator.

## Operate

Use `./deploy.sh` for an update, `docker compose logs -f control` for logs, and
`docker compose down` to stop the stack.

Artifacts live under `data/artifacts`; the SQLite database lives under
`data/control`, and ACME state lives under `data/traefik`.



<!-- docref: begin src=cmd/cadestro/config.go#Config.WebhookURL:341af9cf,internal/maintenance/service.go#Service.InspectSecurity:223fcf91,internal/maintenance/service.go#Service.InspectBackup:d8c2e6fd -->
Set the optional `CADESTRO_WEBHOOK_URL` to an HTTPS endpoint to receive
generic security
and backup-lag notifications. The payload contains only the event name and
occurrence time; control has no email or provider-specific notification
integration.
<!-- docref: end -->

<!-- docref: begin src=deploy/backup.sh#@sqlite-backup:99bc90ed,cmd/cadestro/backup_status.go#runBackupStatus:41ed4e6c -->
Run `./backup.sh` from a host timer at least daily. It takes an online SQLite
`.backup`, then verifies the copy with `integrity_check` and `foreign_key_check`
before atomically publishing `backup-status.json`. It retains seven backups by
default and never touches readiness. Inspect the latest success and current lag
with `docker compose exec control cadestro backup-status`;
`CADESTRO_BACKUP_MAX_LAG` defaults to 26 hours.
<!-- docref: end -->

<!-- docref: begin src=internal/store/reads.go#ListDueDeliveries:bbaaa8a0,internal/store/search.go#Search:3244914e -->
Pending dispatch is ordinary SQLite state. Search uses SQLite FTS5. There is no
broker, projector rebuild, dynamic proxy provider, or auxiliary search process
to operate.
<!-- docref: end -->
