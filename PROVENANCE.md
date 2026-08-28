# Provenance

This monorepo is a **squashed seed** of the Power Manage component
repositories under the product's new name. The seed deliberately discards
upstream git history; this file is the record that makes that loss auditable
rather than silent. The source repositories are archived read-only — any
archaeology (why a line exists, which issue drove it, when a tag shipped) is
answered there, at the SHAs below.

## Source states, exactly as seeded (captured 13 August 2026)

| Module | Source repo | Seeded state |
|---|---|---|
| `server/` | `github.com/manchtools/power-manage-server` | `main` @ `031605db2952ab518b7de9b5765f5bb46fd99431` |
| `agent/` | `github.com/manchtools/power-manage-agent` | local octopus merge `7997d3ce8b448e52317f615ce1a7bc06cea6fb3d` of `main` @ `0f6403efb48ed086b0421c3562964c7519c7744b` + PR #203 (`docs/contributor-guide` @ `871643c5ec2e8b963bb15887d82f5e4694a43364`) + PR #205 (`fix/release-fork-safety` @ `3255d029f23af81fdba421b17de479e1c86c58e9`); both PRs were fully gate-verified and CI-green at seed time |
| `sdk/` → split into `contract/` + `sdk/` | `github.com/manchtools/power-manage-sdk` | PR #337 branch `fix/sys-review-notes` @ `2be1782c4179faca53306120a40942bd871d972b` (main + the July sys-review fixes + review batch, fully gate-verified and CI-green) |
| `web/` | private predecessor web repository (open-sourced with this move) | `fix/security-audit-remediation` @ `c95e97e3921fa33b52180013a5aa98f11a8721d9` |
| docs | `github.com/manchtools/power-manage-docs` | **not seeded by ruling** — the old corpus carried too much legacy and remains only in its archived repository. The later Cadestro documentation was removed during the pre-1.0 core descope and remains on `archive/pre-core-20260828` |

## Uncommitted state at seed time, and its disposition

| Repo | Path | Kind | Disposition |
|---|---|---|---|
| web | `Makefile` | modified (blank-line edit, operator's) | **Not seeded** — cosmetic, left in the source tree |

No other tracked file was dirty in any source repository at seed time.

## What the squash discards, stated plainly

- **Blame and history**: the source repositories' full commit histories. They
  remain browsable in the archived repos.
- **Issue cross-references**: commit subjects referencing
  `manchtools/power-manage-{server,agent,sdk}#N` resolve in the archived
  repos, not here. Still-relevant open issues are re-filed here before the
  archives close. In-code citations use the shorthand `archived <module>#N`
  — `archived server#58` is issue 58 of the `server/` row above — because
  the predecessor repository names are swept out of the source tree; this
  table is what resolves them.
- **Tags and releases**: published tags (including the `v2026.08` RC line)
  stay in the source repos and are not re-pointed.
- **The operator CLI** (`cmd/powermanage`): deliberately not carried — removed
  by ruling, with a capability-parity check against the web UI recorded in
  this repository's history.

## Naming

The seed lands under the predecessor identifiers (module paths, binary names,
proto package, env prefix). The rename to Cadestro identifiers is performed as
reviewable increments on top of this seed, so every rename is separable from
the import it renames.
