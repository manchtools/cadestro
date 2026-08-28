# Stallion audit — cadestro

Date: 2026-08-22
Branch: refactor/full-lean at f58f0ba
Rule set: stallion `AGENTS.md`
Method: read-only static sweep of the whole tree. No gate was executed.

---

## §6 Machine safety — 2 findings

| Location | Rule broken | Fix |
|---|---|---|
| `scripts/verify-all.sh:99`, `:112` | `go test -count=1 ./...` with no `-p 1` — packages run concurrently at GOMAXPROCS | add `-p 1` |
| `agent/scripts/verify.sh:42`, `contract/scripts/verify.sh:43`, `sdk/scripts/verify.sh:50`, `server/scripts/verify.sh:38` | same, all four module gates | same |

No `--memory` or `--cpus` cap appears in any script or in `agent/Makefile`, whose
targets build and run distro containers.

This is the mechanism behind the OOM kills. `verify-all.sh` is strictly
sequential *between* gates, exactly as its header promises, but each gate then
fans out internally.

## §15 Validation — 3 findings

protovalidate is not present. Zero `buf.validate` annotations across all protos;
no `connectrpc.com/validate` import anywhere.

| Location | Rule broken | Fix |
|---|---|---|
| `contract/validate/validate.go` | go-playground/validator via `@gotags: validate:"…"`, injected by protoc-go-inject-tag — 717 tag sites across five protos | the constraint belongs in the schema |
| `server/internal/authoring/handlers.go:55`, `device/handlers.go:108`, `dispatch/handlers.go:74`, `identity/handlers.go:132`, `searchrpc/handlers.go:93` | five hand-written per-handler `validate(ctx, message any)` — the duplication §15 forbids, on a weak type | one interceptor at handler construction |
| `contract/test/protovalidatecoverage/main.go` | a coverage checker for protovalidate, wired into no gate, measuring a thing that does not exist | delete or wire |

Requested explicitly on 2026-08-17: "please add protovalidate from the start".

## §9 Comments — absolute rule, 25,113 violations

- 22,765 in non-generated Go
- 1,839 in proto
- 509 in SQL

Worst files: `agent/internal/executor/integration_test.go` 440,
`contract/client.go` 398, `agent/internal/handler/terminal.go` 292.

The gate scripts are the densest prose in the repo. They explain *why* a check
exists, which is the one thing a chat reply cannot preserve. The rule as written
has no exception, so this is either the largest violation in the tree or the rule
needs a carve-out for shell gates. That decision is the operator's.

## §12 Code generation — 2 findings

`contract/scripts/buf.sh` is exemplary and does exactly what §12 asks. Two
generators escape it.

| Location | Rule broken | Fix |
|---|---|---|
| `contract/Makefile:32` | `protoc-go-inject-tag@latest`, and it rewrites generated source — an upstream release silently changes `gen/go` and fails the drift gate on an unrelated change | pin via `go list -m` like its two neighbours |
| `contract/Makefile:38` | bare `protoc` off PATH, no wrapper, no fail-closed check | resolve like `buf.sh` does |

Output directories are emptied before regeneration and the pipeline is explicitly
sequenced rather than using prerequisites. Both correct.

## §13 sqlc / SQLite — 2 findings

Single schema source (`schema.sql`, no migrations directory) — compliant.
Pragmas correct at `server/internal/store/store.go:165-168`.

| Location | Rule broken | Fix |
|---|---|---|
| 63 sites, e.g. `server/internal/store/search_documents.go:81`, `policy_results.go:46` | raw SQL in Go outside `queries/` | move to sqlc, or record FTS/dynamic search as a stated exception |
| `server/internal/testdb/sqlite.go:42` | test DB omits `journal_mode` and `synchronous` — tests run under different durability than production | match the production pragma string |

## §8 Clean break — 3 findings

| Location | Rule broken | Fix |
|---|---|---|
| `contract/proto/cadestro/v1/control.proto:27,45,56,64,447,461` | 8 `reserved` statements including named tombstones `"one_time"`, `"owner_id"` — forbidden pre-1.0 | remove and renumber |
| `contract/client.go:326` | `WithH2C()` cleartext transport option, zero callers | delete |
| `web/src/lib/dev/skip-auth.ts` + `web/src/routes/+layout.svelte:4` | auth bypass, self-labelled TEMPORARY with its own removal note | remove |

skip-auth is compile-time eliminated by `import.meta.env.DEV`, so it is not a
production hole. It is dead scaffolding, not a vulnerability.

## §10 Go — 2 findings

- 65 `panic()` in non-test library code, e.g. `contract/validate/validate.go:11`.
- `server/internal/manifest/compiler.go:149` uses `context.Background()` in a
  request path. The agent has an AST guard with an allowlist at
  `agent/internal/archtest/context_background_test.go`; the server has none.

622 of 630 `fmt.Errorf` sites wrap with `%w`.

## §14 Svelte and §9 weak types — lower severity

- 90 `$effect` uses, not individually classified.
- `web/src/routes/(app)/devices/fleet-data.ts` writes a store from `load`.
- 65 `map[string]any` in Go, e.g. `server/internal/idp/oidc.go:227`.
- 9 `any` in TypeScript.

---

## Refuted — checked and cleared

- `allow_downgrade` at `contract/proto/cadestro/v1/actions.proto:143` is a
  package-manager feature, not a compatibility shim.
- Enum non-`_UNSPECIFIED` zeros and shared request/response types are recorded
  exceptions in `contract/proto/buf.yaml`, which §11 states is the ruling.
- All 13 TODO/XXX hits are false positives: `mktemp` templates, the
  `context.TODO` detector, test fixtures.
- No AI self-attribution in git history. The three apparent hits were the phrase
  "regenerated with `make generate`".
- Agent `context.Background()` uses are governed by the archtest allowlist.

## Counts

| Section | Findings |
|---|---|
| §9 comments | 25,113 |
| §9 weak types | 74 |
| §10 Go | 66 |
| §13 sqlc | 65 |
| §15 validation | 3 (systemic) |
| §8 clean break | 3 |
| §12 codegen | 2 |
| §6 machine safety | 2 (systemic) |
| §14 Svelte | 2 |

## Could not check

- **Generated drift.** The only valid check is regenerate then
  `git diff --exit-code`, which writes to the tree. Read-only forbade it.
  Untested.
- **All five gates.** Not run. Running them is the exact fan-out §6 flags, and a
  gate that did not execute must never be reported green.
- **`svelte-check`.** Not run.
- **The 90 `$effect` uses.** Counted, not individually classified as
  derived-state misuse versus legitimate DOM work. That needs reading each one.

## Fix these three first

1. **`-p 1` in five files.** One flag each, and it ends the failure that has cost
   an OOM six times and a hard power cycle.
2. **protovalidate.** Every day it stays absent, more `@gotags` constraints
   accumulate that will have to be migrated. 717 already.
3. **`protoc-go-inject-tag@latest`.** A tool that rewrites generated output at an
   unpinned version is a green gate over a tree nobody built. It fails on an
   unrelated day and looks like an unrelated change broke it.
