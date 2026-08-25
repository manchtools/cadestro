# Backup and restore

The database is the source of truth for devices, actions, assignments, results,
and audit events. Back it up with the script you schedule below.

---

## Backing up the database

```bash
/opt/cadestro/backup.sh
```

<!-- docref: begin src=server/deploy/backup.sh#@sqlite-backup:c19c264f -->
It uses SQLite's **online backup API**, not a file copy. A copy of a live
WAL-mode database is not a database — it is a file plus two sidecars in an
unknown relationship — and the backup API produces one consistent, self-contained
file from a running server.

It then earns the word "verified" rather than assuming it:

1. the database file must exist and contain an applied Goose migration;
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
filename pattern, so it cannot delete the status document or another file in the
directory.

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

<!-- docref: begin src=server/internal/backupstatus/status.go#StatusFilename:c2db02bf -->
The status document is only ever replaced after SQLite has validated a snapshot,
which is what makes its timestamp meaningful: it records the last time a backup
was *verified*, not the last time one was attempted.
<!-- docref: end -->

Staleness is `CADESTRO_BACKUP_MAX_LAG`, 26 hours by default — chosen to let a
daily backup run late without alarming.

> **One precision.** The status check compares the artifact's **size**, not its
> hash. The recorded digest is validated for shape but is not recomputed against
> the bytes on every status read. It is a freshness and consistency check, not a
> continuous integrity check of your backup artifact.

<!-- docref: begin src=server/internal/maintenance/service.go#Service.InspectBackup:d8c2e6fd -->
Control also inspects backup posture itself every 15 minutes, records the result
as an audit effect, and fires the configured webhook when it goes stale — all
**without changing application readiness**. A control plane that is serving
correctly is not taken out of rotation because a backup timer failed; it tells
you instead.
<!-- docref: end -->

---

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
| `secrets/session-signing.pem` | existing sessions |
| `certs/` | the CA **every enrolled device pinned** — lose it and you re-enroll the fleet |
| `data/artifacts` | uploaded artifacts, not in the database |
| `data/backups` | verified database backup artifacts |

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

- **Goose owns schema changes.** Starting control applies any pending ordered
  migrations before serving requests. See [upgrades](upgrade.md).

### What is unproven

No code exercises a restore. Anything beyond "stop control, put the artifact at
the database path, start control" is inference, so if you depend on being able
to restore, **test it on a spare machine before you need it.**

---

## Where to go next

- [Security model](security-model.md#4-audit-guarantees) — what the audit log
  records and how it is protected.
- [Installation](installation.md) — deployment prerequisites.
- [Upgrades](upgrade.md) — why a backup is a prerequisite.
