# Upgrades

## The honest statement first

**Cadestro is pre-1.0. There is no automatic schema migration machinery.** A
schema change requires a clean reinstall; startup never guesses how to
transform old data.

That is not a gap waiting to be filled in a later sprint; it is the current
design, and this page describes what the code actually supports rather than what
a mature product would.

<!-- docref: begin src=server/internal/store/store.go#initializeSQLite:05f20fae -->
Schema handling is a three-way decision at startup, and there is no migration
runner anywhere in the server. An empty database gets the baseline schema
applied in one transaction. A database already at the current version is opened.
**Any other version is refused** with an unsupported-schema-version error — not
migrated, not best-effort upgraded, refused.
<!-- docref: end -->

So: within a schema version, updating is a container image pull. Across one,
reinstall.

---

## Updating within a version

<!-- docref: begin src=server/deploy/compose.yml#@deployment-services:c3bfad13 -->
The control and web images share a single tag variable, because both are
released from one repository under one version. They are updated together by
construction — you cannot accidentally run a control plane and a UI from
different releases. The ingress is pinned to an exact version independently.
<!-- docref: end -->

The procedure:

```bash
cd /opt/cadestro
$EDITOR .env          # set IMAGE_TAG to the release you want
./deploy.sh
```

`deploy.sh` requires an existing `.env`, re-runs `setup.sh`, validates the
compose configuration, pulls the three services by name, brings the stack up
waiting for health, and prints the resulting status.

Two consequences worth internalising:

- **`setup.sh` re-renders the configuration on every deploy.** Hand edits to
  `config/control.env` or `config/web.env` are overwritten. Change `.env`, or
  change `setup.sh` — do not change its output.
- **The installer refuses to run against an existing install** and points you at
  `deploy.sh` by name, so you cannot accidentally re-run the first-time flow over
  a live deployment.

### Pinning

`IMAGE_TAG` defaults to `latest`, which is fine for evaluation and wrong for
production. The installer derives the initial pin from the release tag you gave
it, and nothing writes to it afterwards — so once installed, your version is
whatever you last set, and it will not drift under you.

One footgun the deployment's own smoke test documents: Compose substitutes from
the **process** environment before the env file. An exported but empty
`IMAGE_TAG` in your shell or CI resolves to `latest` and silently pulls
something other than the pin in `.env`.

### Rolling back

Set `IMAGE_TAG` to the previous release and run `./deploy.sh` again. This works
as long as both releases share a schema version — the same condition as upgrading
forward, in the other direction. Combine it with a
[restore](backup-restore.md#restoring) if the newer version wrote data the older
one cannot read.

---

## Across a schema version

Reinstall across a schema version. Concretely:

1. Take a [verified backup](backup-restore.md) and copy `certs/`, `secrets/`,
   and `data/artifacts` off the machine alongside it.
2. Install the new release fresh.
3. Re-enroll devices.

<!-- docref: begin src=server/internal/store/store.go#NewWithoutMigrations:e20a97cf -->
Restoring the old database into the new release is not a migration and will not
be treated as one: the store's non-creating open path requires the exact current
schema version and refuses anything else. A database from a different schema
version will not open, and the failure is at startup rather than halfway through
a request.
<!-- docref: end -->

**Devices must be re-enrolled** unless you carried `certs/` across, because the
CA is what every enrolled device pinned. See [enrollment](enrollment.md).

---

## The agent

Agents update independently of the control plane, through a policy rather than
through this procedure. See
[the agent self-update capability](capabilities.md#agent-self-update): the agent
verifies a publisher-signed checksum manifest with an embedded key, refuses to
install an older version unless downgrade is explicitly allowed, and runs the
candidate binary's self-test before swapping it in.

That means you do not have to reinstall agents to update them — but you do have
to re-enroll them if you rebuild the control plane's CA.

---

## Where to go next

- [Backup and restore](backup-restore.md) — do this before any upgrade.
- [Installation](installation.md) — the clean-install procedure.
