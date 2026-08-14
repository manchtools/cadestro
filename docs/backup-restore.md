# Backup and restore

There are **two** separate things to keep, and conflating them is the mistake
this page exists to prevent:

1. **The database** — every device, action, assignment, and result. Backed up by
   a script you schedule.
2. **The audit archive** — retained audit evidence, written by control itself
   when it prunes old audit rows, and required to be on a different filesystem.

Losing the first costs you the deployment. Losing the second costs you the
evidence about the deployment, which is the harder thing to reconstruct.

---

## Backing up the database

```bash
/opt/cadestro/backup.sh
```

<!-- docref: begin src=server/deploy/backup.sh#@sqlite-backup:99bc90ed -->
It uses SQLite's **online backup API**, not a file copy. A copy of a live
WAL-mode database is not a database — it is a file plus two sidecars in an
unknown relationship — and the backup API produces one consistent, self-contained
file from a running server.

It then earns the word "verified" rather than assuming it:

1. the database file must exist and be at the expected schema version;
2. the snapshot is taken to a temporary name in the destination directory;
3. `PRAGMA integrity_check` on **the copy** must return exactly `ok`;
4. `PRAGMA foreign_key_check` on the copy must return nothing;
5. the artifact must be non-empty, and it is hashed;
6. only then is it atomically renamed into place;
7. and the status document is written to a temporary file and atomically moved
   over the previous one.

Every one of those failures exits non-zero without publishing anything. A
corrupt snapshot never becomes the newest backup.
<!-- docref: end -->

<!-- docref: begin src=server/deploy/backup.sh#cleanup:482c0a5a -->
An exit trap removes the temporary artifact and temporary status file and
re-exits with the original status, so an interrupted run leaves no partial file
behind for the next run to trip over.
<!-- docref: end -->

Retention is `BACKUP_KEEP`, default 7, validated as an integer between 1 and
365. Pruning is by reverse-sorted filename, which is reverse-chronological
because the timestamps are fixed-width — and it only ever matches the snapshot
filename pattern, so it cannot delete the status document or an archive object
sharing the directory.

### Scheduling it

**No timer unit is shipped.** The script is the mechanism; scheduling is yours:

```ini
# /etc/systemd/system/cadestro-backup.service
[Service]
Type=oneshot
ExecStart=/opt/cadestro/backup.sh

# /etc/systemd/system/cadestro-backup.timer
[Timer]
OnCalendar=daily
Persistent=true
[Install]
WantedBy=timers.target
```

Getting the resulting files **off the machine** is also yours. The deployment
gives you a verified artifact in a directory; it does not replicate it.

---

## Knowing your backups are current

<!-- docref: begin src=server/cmd/cadestro/backup_status.go#runBackupStatus:41ed4e6c -->
```bash
docker compose exec control cadestro backup-status
```

Exit 0 means a valid, fresh backup. **Exit 1 means a missing, invalid, or stale
one** — which makes it directly usable as a monitoring check. The JSON status
document is printed on stdout either way, so a failing check still tells you
*what* is wrong; the human-readable diagnosis goes to stderr.
<!-- docref: end -->

<!-- docref: begin src=server/internal/backupstatus/status.go#Read:7b336091 -->
"Valid" is a real check, not a file-exists test. The marker must be a regular
file — a symlink is refused — within a size bound, decoding strictly with no
unknown fields and no trailing content, at the expected version, with a
completion time that is neither zero nor implausibly in the future, with an
artifact name that is a bare filename (so it cannot point outside the
directory), and with the named artifact actually present **and matching the
recorded size**.
<!-- docref: end -->

<!-- docref: begin src=server/internal/backupstatus/status.go#StatusFilename:7eb37ef1 -->
The status document is only ever replaced after SQLite has validated a snapshot,
which is what makes its timestamp meaningful: it records the last time a backup
was *verified*, not the last time one was attempted.
<!-- docref: end -->

Staleness is `CADESTRO_BACKUP_MAX_LAG`, 26 hours by default — chosen to let a
daily backup run late without alarming.

> **One precision.** The status check compares the artifact's **size**, not its
> hash. The recorded digest is validated for shape but is not recomputed against
> the bytes on every status read. It is a freshness and consistency check, not a
> continuous integrity check of your backup archive.

<!-- docref: begin src=server/internal/maintenance/service.go#Service.InspectBackup:d8c2e6fd -->
Control also inspects backup posture itself every 15 minutes, records the result
as an audit effect, and fires the configured webhook when it goes stale — all
**without changing application readiness**. A control plane that is serving
correctly is not taken out of rotation because a backup timer failed; it tells
you instead.
<!-- docref: end -->

---

## The audit archive

<!-- docref: begin src=server/internal/archive/archive.go#ArchiveStore:66c8a29a -->
The archive holds integrity-sealed audit anchors and retained chain prefixes on
the operator's off-host backup mount. It is a streaming store by design — a
retained prefix can be large — and the interface has `put`, `get`, and `list`.

**There is no delete.** Nothing in the archive is ever removed by the
application.
<!-- docref: end -->

<!-- docref: begin src=server/internal/store/audit_archive.go#Store.WriteAuditPrefix:0e66b341 -->
An archived prefix is deterministic JSON-lines: a header line naming the stream,
the boundary, the boundary hash and the prior checkpoint, then one line per row
in strict chain order — with a gap check while writing and a count
reconciliation afterwards. It is a plain text format that can be read and
verified without Cadestro.
<!-- docref: end -->

<!-- docref: begin src=server/internal/archive/fs.go#filesystem.Put:49f203c8 -->
Writing is ordered for crash safety: the content is streamed and hashed to a
temporary file in the same directory, fsync'ed; the checksum sidecar is written
and fsync'ed **first**; then the data file is atomically renamed into place; then
the directory is fsync'ed. A reader therefore never sees an object whose data is
incomplete, and a present object always has a durable seal beside it.
<!-- docref: end -->

<!-- docref: begin src=server/internal/maintenance/service.go#recurring:4c272529 -->
Three audit jobs run on their own schedules: verification hourly, anchoring
every 15 minutes, and retention daily.
<!-- docref: end -->

### Why the separate filesystem

Because the archive is evidence *about* the database. Evidence that shares a
failure domain with what it attests is not independent evidence — one bad disk,
one bad `rm`, one ransomware run takes both. See
[installation](installation.md#the-archive-mount) for how to satisfy it and
[the security model](security-model.md#retention-and-archive) for what the
archive proves.

Note the honest limit: the enforced property is *different filesystem*. A second
local disk passes. Getting it genuinely off-host is yours to arrange.

### Backing up the archive

Include the archive mount in whatever off-host copy you already run. There is no
Cadestro-side replication.

---

## Restoring

**There is no restore script.** This is a deliberate statement of what the code
supports, not an oversight this page is glossing over. What follows is the
procedure the code makes possible, and the constraints it imposes.

### What a complete restore unit is

The database alone is not enough:

| Keep | Why |
|---|---|
| `data/control/control.db` | the deployment |
| `secrets/encryption.key` | without it, every stored secret is unreadable |
| `secrets/sealing.key` | field sealing to and from agents |
| `secrets/session-signing.pem` | existing sessions |
| `certs/` | the CA **every enrolled device pinned** — lose it and you re-enroll the fleet |
| `data/artifacts` | uploaded artifacts, not in the database |
| `data/backups` | the audit archive |

### The procedure

```bash
cd /opt/cadestro
docker compose stop control

cp /path/to/sqlite-<stamp>.db data/control/control.db
rm -f data/control/control.db-wal data/control/control.db-shm

docker compose start control
docker compose logs -f control
```

The sidecar removal matters: the snapshot is a single consolidated file, and
leaving a WAL and shared-memory file from the previous database next to it
invites SQLite to reconcile two unrelated states.

<!-- docref: begin src=server/internal/store/store.go#prepareSQLiteFile:736f513d -->
You do not need to fix the file's ownership or mode by hand — control opens it
read-write and chmods it to owner-only on every open.
<!-- docref: end -->

### Constraints the code imposes

- **Schema version must match.** A snapshot from a different release's schema
  will not open. See [upgrades](upgrade.md).
- **Restore the database and the archive as a matched pair.** A restored
  database whose audit checkpoints reference archive objects that are not
  present will fail audit verification and block retention — both fail closed,
  by design, because a missing prefix is indistinguishable from a deleted one.
- **The archive must still be on its own filesystem** or control will not start.

### What is unproven

No code exercises a restore. Anything beyond "stop control, put the artifact at
the database path, start control" is inference. In particular, nothing verifies
that a restored database with a mismatched archive is recoverable — so if you
depend on being able to restore, **test it on a spare machine before you need
it.**

---

## Where to go next

- [Security model](security-model.md#4-audit-guarantees) — what the audit
  archive proves and how.
- [Installation](installation.md) — the archive mount requirement.
- [Upgrades](upgrade.md) — why a backup is a prerequisite.
