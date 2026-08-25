# Upgrades

## The honest statement first

**Cadestro is pre-1.0, and the control plane embeds Goose as its schema
migrator.** Startup applies every pending ordered migration before serving
requests. The current pre-1.0 history is a single squashed baseline because
that history has not been released.

<!-- docref: begin src=server/internal/store/store.go#migrateSQLite:9c229738 -->
Schema handling at startup is an idempotent Goose apply. An empty database gets
the current baseline, and an existing database receives every pending ordered
migration. sqlc reads the same migrations directory, so generated queries and
the deployed schema share one source of truth. Before 1.0, unreleased history
may be squashed into a new baseline. After a schema is released, its migration
file is immutable: each later schema change is a new migration with tested
`Up` and `Down` sections.
<!-- docref: end -->

So: updating is a container image pull followed by the embedded Goose apply.

---

## Updating within a version

<!-- docref: begin src=server/deploy/compose.yml#@deployment-services:9a1eca5a -->
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

Set `IMAGE_TAG` to the previous release and run `./deploy.sh` again only when
that release can read the current schema. A migration's `Down` section is the
tested rollback definition, but deployment does not silently downgrade a
database. If the older binary cannot read the newer schema, restore the verified
backup taken before the upgrade, then deploy the older release.

---

## Across a schema version

Goose upgrades the schema in place. Concretely:

1. Take a [verified backup](backup-restore.md) and copy `certs/`, `secrets/`,
   and `data/artifacts` off the machine alongside it.
2. Set `IMAGE_TAG` to the new release.
3. Run `./deploy.sh`; control applies pending migrations before becoming ready.

<!-- docref: begin src=server/internal/store/store.go#NewWithoutMigrations:f91d0fc3 -->
The embedded runner is the migration path; it does not guess or mutate schema
outside the ordered Goose files. The non-creating bootstrap command deliberately
does not run migrations and therefore requires the database to already be
current, while the control server's normal startup runs them automatically.
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
