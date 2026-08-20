# Cadestro documentation

Written from the code in this repository. Every statement about behaviour is
anchored with [docref](https://github.com/manchtools/open-docref) to the source
that proves it, so a change to the code fails the page that describes it.

## Start here

**[The policy model](policy-model.md)** — read this first. Desired state,
manifests, durable delivery, maintenance windows, one-shot dispatch, offline
autonomy, and why compliance never remediates. Everything else is an application
of it.

## Running it

| | |
|---|---|
| **[Installation](installation.md)** | Standing up control: DNS, TLS, the compose stack, and first login. |
| **[Enrollment](enrollment.md)** | Getting devices in: tokens, the installer, CA pinning, the mTLS stream, renewal, and active-certificate admission. |
| **[Backup and restore](backup-restore.md)** | What to keep, how the verified backup works, and the restore procedure the code actually supports. |
| **[Upgrades](upgrade.md)** | Image pulls within a version; clean reinstall across one. Pre-1.0 reality, stated plainly. |

## Reference

| | |
|---|---|
| **[Capability reference](capabilities.md)** | Every action type, its parameters, and the real tool behind it. |
| **[Security model](security-model.md)** | Trust boundaries, identity, device PKI, audit guarantees, secret handling — and an explicit list of what the code does *not* establish. |

## Elsewhere in the repository

- [`../README.md`](../README.md) — what Cadestro is, in 30 seconds.
- [`../CONTRIBUTING.md`](../CONTRIBUTING.md) — building, testing, and the
  documentation workflow.
- [`../SECURITY.md`](../SECURITY.md) — reporting a vulnerability.
- [`../LICENSING.md`](../LICENSING.md) — the per-module licence split.
- [`../server/deploy/QUICKSTART.md`](../server/deploy/QUICKSTART.md) — the
  deployment tree's own operational reference.
- [`../agent/README.md`](../agent/README.md) — the device agent in depth.
- [`../sdk/docs/`](../sdk/docs/) — the system capability SDK.

## A note on how to read this

Where the code proves something narrower than a reader might assume, these pages
say so rather than rounding up — in **Caveat** callouts inline, and in the
security model's [limits section](security-model.md#limits-stated-plainly).
Those parts are load-bearing. They are what makes the rest trustworthy.
