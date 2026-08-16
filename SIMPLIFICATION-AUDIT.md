# Cadestro Simplification Audit

Second, deeper read-only pass over the tree at `99ab489`. The first pass found real problems
but over-claimed deletions and proposed an incomplete pull-policy design; an intervening
correction withdrew the unsafe conclusions. This revision closes the lifecycle gaps that both
earlier passes left open — the reconciliation and recurring-run model was traced end to end this
time — re-verifies every retained finding against source, and states the remaining unknowns
plainly. Nothing was implemented; the only file written is this one.

Verdict vocabulary: **DELETE | MERGE | SIMPLIFY | FIX | KEEP | ADD | PRODUCT DECISION**.
Confidence: **confirmed** (the complete relevant path was traced, usually by two independent
agents and re-verified by a deep-reasoning pass) | **strong** | **tentative**.
`ZERO INTERNAL CALLERS` is a reason to investigate, never by itself a reason to delete a public
RPC or an intentionally reusable SDK capability.

---

## 1. Executive verdict

The headline of this audit is **not a line count.** The load-bearing finding — re-verified three
independent ways — is that **Cadestro has no policy-removal path and no recurring-run identity at
any layer, and assigning policy to a group currently does nothing.**

- Assigning an ActionSet to a DEVICE_GROUP writes one inert `assignments` row. The only code that
  turns an assignment into device work, `assignedManifests`, is reachable solely through the
  `DispatchAssignedActions` RPC, which has **zero production callers**. `Sync` re-sends
  pre-existing delivery rows and never consults `assignments`.
- There is **no removal message on the wire** (the `ServerMessage` oneof carries no
  remove/uninstall/expire payload — re-verified by reading it whole), **no agent operation that
  deletes a stored manifest**, and **no "still assigned" gate** on the agent's due-check — so any
  locally installed recurring schedule fires forever regardless of server state.
- There is **no execution-run identity**: the wire has occurrence-*position* identity but no
  execution-*firing* identity, and `executions` is a single row updated in place with no history
  table. A recurring policy's second, differing result is rejected (`ErrInvalidTransition`, before
  the replay branch is even reached) and — because the agent's result send is fire-and-forget —
  **silently dropped**. (This is latent today only because production installs zero recurring
  schedules, for the same reason: the recurring path is the dead RPC.)

Every large architectural deletion in this report is **downstream of building that missing
model** (a stable policy identity, a per-firing run identity, a set-difference reconcile, and
history rows) and proving it with failing-first tests. Deletion is a *consequence* of the model
passing tests, not an independent win. **That target model is a *direction with named, unsolved
open problems* (§4, O1–O5), not a finished blueprint** — the installed-policy identity,
cross-sync result correlation, the result-acceptance guarantee, removal safety during execution,
and the five ordinary-action push RPCs must each be designed and answered before Phases 4–5 are
safe to implement.

The conclusion, split as the brief requires:

- **Confirmed safe cuts (validated now):** **1,044 lines** of genuinely private, zero-caller
  dead code (measured, build-aware, re-verified per item — §6), plus the security/reliability
  floor fixes (§11), led by the critical privilege escalation. These are the only cuts and fixes
  that are validated today.
- **Conditional architectural cuts (unknown until the model exists + its tests pass):** the
  assigned-dispatch path, the policy push/delivery state machine (~1,257 test lines), and the
  device-wide maintenance-window union. Real, but not safe to remove before Phase 4/5.
- **Fixes:** the privilege escalation (two enforcement points, §11 S1), unsigned-package default,
  authorization-denial auditing, SSO redirect pinning, recurring-job re-arm, agent-clamped
  compliance, doas fail-closed, enrollment-token bounds, identity last-link deletion, targeted
  FILE validation, rotation-interval-zero, security-alert forensic persistence.
- **Intentional capability to preserve (not dead):** every implemented SDK package including
  `sdk/sys/remote`; the five exposed public RPCs; and the tests that protect them.
- **Product decisions:** the five public RPCs' lifecycle; `respect_maintenance_window`; the
  write-only proto fields; ordered-definition STOP scope; the job runner's multi-instance future.
- **Rejected claims (§20):** SDK-dead-for-no-importer; RPCs-dead-for-no-web-caller;
  content-hash-solves-reconciliation; `time.Ticker`-is-automatically-simpler; the
  `deliveries`-table-is-immutable; three of four "removable" web deps; the FILE-write privilege
  escalation; and a "live secret-in-logs" claim.

<!-- docref: begin src=sdk/README.md:17ee2a8e,sdk/sys/remote/doc.go:47e79d04 -->
The SDK describes itself as reusable, MIT-licensed capability code intended for other tools,
and `sdk/sys/remote` explicitly documents its Git/S3/HTTP implementations as a reusable
building block for future and external consumers. Internal reachability therefore cannot be
used as its deletion criterion.
<!-- docref: end -->

---

## 2. The intended product model (restated)

The administrator's mental model the target must match:

- **One desired-state model.** An assignment is declarative desired state; creating, changing, or
  deleting one — or changing target membership — affects a device on its **next sync**.
- **One authoritative resolver** expanding all four target kinds (DEVICE, DEVICE_GROUP, USER,
  USER_GROUP), applying mode precedence (Excluded > Uninstall > Required > Available) and
  containment (Definition > ActionSet > Action), resolving conflicts/duplicates once, preserving
  declared order, attaching schedule and the applicable maintenance constraint, feeding **both**
  preview and sync.
- **Sync is reconciliation, not insert-only delivery.** It handles newly-assigned, unchanged,
  changed, deleted, membership-removed, changed-schedule/window/contents, stale-local, restart,
  and interrupted-reconcile.
- **Policy identity and run identity are different concepts.** A stable identity/version answers
  "what desired policy is installed?"; a fresh run identity answers "which firing produced this
  result?" A recurring policy runs twice → two valid histories, no replay conflict.
- **Normal policy is pulled;** control→agent push is reserved for instant actions and terminal.
- **Compliance is server-derived** from evidence and server-controlled time; prefer read-time
  derivation over a sweep.
- **Identity lifecycle follows provider links** — the last link removed deletes the user,
  regardless of original provisioning source, keeping the bind-time takeover guard.
- **Defaults match ordinary expectations;** proto3 zero values must not contradict them.
- **One canonical representation per source-of-truth list.**

---

## 3. Confirmed architecture map

Traced end to end this pass (server rows → resolver → identities → tx boundaries → agent store →
crash/replay → result correlation → history). The reality, by scenario:

| Scenario | Today |
|---|---|
| Direct DEVICE assign, then sync | `assignments` row written; `Sync` re-sends `deliveries WHERE state IN ('PENDING','PUSHED')`; no delivery exists from assigning → device receives **nothing** |
| DEVICE_GROUP assign; add/remove device | SQL expansion of all four target kinds is correct and uncached; but nothing consumes it on sync → a joining device gets nothing, a leaving device keeps what it had |
| Change assigned set/definition contents | recompiled only if re-dispatched; agent `RecordManifestDelivery` is insert-once, and a changed manifest under a reused `delivery_id` is **hard-rejected** |
| Delete an assignment | soft-deleted row drops from future resolves, but nothing consumes "absent"; the already-inserted delivery keeps sending and the local schedule fires forever |
| Empty a referenced container | `ErrEmptyManifest` aborts the **entire** dispatch batch (zero tests either direction) |
| Unchanged 2nd sync | sends nothing — idempotent by accident of the transport queue emptying, not by desired-state comparison |
| Same policy twice, different results | run 2 hits `ErrInvalidTransition` (delivery no longer `ACKED_RECEIPT`) before the replay branch; agent can't observe the rejection → **silently dropped**; server frozen at run 1 |
| Remove ALL policy | **impossible today** — no removal signal, no agent reconcile-delete; every recurring schedule fires forever |
| Instant action during sync | genuine instant actions (REBOOT/SYNC) are one-shot, window-exempt, same transport; the delivery/execution layer is policy-agnostic — but note the five `Dispatch*` RPCs (§7) push *ordinary* content through that same one-shot path, which is the thing to rule on, not a clean instant/policy split |
| Read current + history | current = the singleton `executions` row; **history is not representable**; compliance materialized write-time on the agent's clock, never re-evaluated |

<!-- docref: begin src=agent/internal/store/manifest.go#Store.RecordManifestDelivery:b40ecef4,agent/internal/store/manifest.go#Store.BeginManifestRun:869363a8 -->
The agent currently records a manifest delivery by inserting it once and accepts only an
identical-byte replay for the same delivery ID. There is no corresponding removal or
full-set reconciliation operation. For every later scheduled run, it resets the same stored
occurrence rows back to pending.
<!-- docref: end -->

**Marked smells (traced, one line each):** two divergent resolvers (preview dedups by action id;
`assignedManifests` only dedups siblings in the compliance loop — the `emittedActions` seam is
written in three loops, read in one); a **security divergence** — preview does no per-source
visibility check while dispatch gates each source behind `*VisibleToCaller`; the `assignments`
table has a `sort_order` column the resolution query never selects, so cross-source declared
order is silently lost; handler and domain logic share one Go package (`authoring`/`dispatch`)
with `controlrpc` a mux assembler, so nothing structurally stops SQL in a `*_handlers.go`.

---

## 4. Desired-policy reconciliation and execution-run model

This is the section the earlier passes could not write. **A content hash plus insert-if-absent
does not solve reconciliation** — it has no delete (so removal and remove-all stay broken), and
the only content available to hash includes the non-deterministic `SealedValue` (so it churns
forever on re-seal). Both dead ends were re-confirmed in source.

**This section is a direction with named open problems, not a finished blueprint.** It states the
intended model and the specific questions that must be designed and answered — and tested with
failing-first tests — *before* any of the conditional architectural deletions (§5/§16 Phase 5) is
safe. Do not implement Phases 4–5 from this section as written; treat the "Open problems" list
below as the gate.

**Governing principle — resolve on demand, never prebuild.** The policy/manifest tree is computed
for a device **on demand, at its check-in**, and is not materialized or persisted server-side. An
assignment, membership, definition, schedule, or window change writes only its own authoring row
and triggers **no** rebuild: there is no fleet reconciler, no per-device generation counter, no
cached/materialized policy tree to invalidate, and no prebuilt manifest or delivery row. Building
policy eagerly on every change is exactly the overhead this design removes; the resolve is a
transient, indexed computation run once per sync. The server's only durable per-device policy state
is the lightweight record of *what a device pulled* and the *execution results it later reports*.

1. **Server: `Sync` resolves on demand instead of re-sending.** `Sync` runs `ResolvePlan(deviceID)`
   transiently and returns the complete, closed desired set — computed at that moment, not fetched
   from a prebuilt store. Each item carries a **stable installed-policy identity**, a **revision**
   computed **pre-seal** during the same on-demand resolve (a fingerprint, not a materialized
   artifact), and the compiled, per-device-sealed occurrences, schedule, on_failure, and (Phase 5)
   window. **`ManifestProvenance` is a *candidate part* of that identity, not the whole of it** —
   see Open problem O1: it carries only definition/action-set/action IDs (`agent.proto:227`) and
   does not identify the assignment, target path, compliance-policy source, ordering source, or
   window, and `assignments.sql` deliberately collapses distinct paths with `SELECT DISTINCT
   source_type, source_id, mode`. The identity must be designed, not assumed.

2. **Agent: a real 3-way reconcile keyed on the installed-policy identity** (`ReconcilePlan(desired)`):
   **ADD** (in desired, not local) → install; **CHANGE** (both, revision differs) → replace, same
   revision → no-op with no byte comparison and no churn; **REMOVE** (local, absent from desired) →
   remove the local rows — the operation that does not exist today. Removal is a set difference over
   a stable key, so an empty desired set removes everything ("remove-all" becomes a testable
   invariant). **But an immediate cascading DELETE is unsafe** (Open problem O4): deleting a manifest
   cascades into its occurrences and reboot markers (`001_initial_schema.sql:14`), and a sync could
   erase recovery state *while an occurrence is executing* — after its side effect but before its
   result is persisted. Removal must be a **retire-then-defer** (tombstone the policy, stop scheduling
   new runs, delete only once no run is in flight), and there is a product decision (O4) about whether
   unassignment *cancels* an in-progress run or lets it finish and report.

3. **Execution-run identity — results recorded on report, not prebuilt at sync.** The agent already
   computes a per-firing run start in `BeginManifestRun` (consumed only for duration). Mint a `run_id`
   (ULID) there and stamp it on every `ActionResult`/`ManifestResult`; add the `run_id` wire fields.
   The server pre-creates no execution row; it records a result **when the agent reports one**, in a
   run-history evidence table. **`run_id` alone does not repair the result protocol** — two coupled
   problems remain (Open problems O2, O3): (O2) the compiler mints **fresh random** manifest and
   occurrence IDs on every compile (`compiler.go:79,211,374`), so if the agent keeps its old installed
   manifest under an unchanged revision while the server re-resolves new IDs, later results cannot be
   correlated back to a policy without either **stable/derived identifiers** (deterministic from the
   installed-policy identity, not `ulid.Make()`) or a **durable server-side pulled-manifest→identity
   mapping** — the "one new history table" does not by itself solve this; and (O3) the current path
   requires an *ACKed delivery* (`result.go:72`) and the first `ManifestResult` *terminalizes* the
   delivery (`state.go:274`), and there is **no application-level result acknowledgement** — the agent
   marks a result synced after the socket send succeeds (`runtime.go:293`) even if the server then
   rejects it. Moving the replay gate to `(identity, run_id)` requires also removing the ACKed-delivery
   precondition and the terminalization, **and** either adding a correlated acceptance-ack or
   *explicitly accepting possible silent loss*. Truncating an oversized error (C6) does not provide the
   general guarantee.

4. **Secret handling without hashing ciphertext.** Compute the policy **revision over pre-seal
   authoring inputs** (action rows, schedule, on_failure, mode, membership) — never the sealed bytes,
   which are non-deterministic. A separate per-secret version counter is **speculative**: the existing
   authoring revision / `updated_at` on the action row very likely already changes when a secret is
   edited, which is enough to bump the revision. Prove the existing signal is insufficient before
   adding a new counter.

<!-- docref: begin src=server/internal/execution/result.go#Service.ApplyActionResult:6e334043,server/internal/delivery/state.go#Service.Complete:736a52cf -->
The server currently identifies an execution by the authored occurrence ID, records a
terminal result once, rejects a different later terminal result as a conflicting replay,
and terminalizes the delivery after its manifest result. Reusing stable delivery and
occurrence identities for a second scheduled run therefore cannot represent run history.
<!-- docref: end -->

**Target data-model shape (on-demand, nothing prebuilt) — contingent on O1/O2.** The server stores
**no materialized per-device policy or manifest tree** — under on-demand resolution there is nothing
to prebuild, cache, or invalidate. It keeps only: (a) a lightweight **check-in / pulled-state** record
per device (its installed-policy identity set + revision, so the next sync's diff is cheap — "track
check-ins and what was pulled"); and (b) **execution results (evidence)** the agent reports, in a
run-history table. The concrete pieces are: a **stable installed-policy identity** (to be *designed*
per O1 — provenance is a candidate part, not the whole); **+1 wire field** (`run_id`); **+1 server
table** (policy-run history / evidence); **+1 concept** (policy revision, computed on-demand pre-seal
during each resolve); **+1 agent op** (`ReconcilePlan` with retire-then-defer removal). This
**replaces** — it does not retain — the policy half of the current push machinery: the per-device
`deliveries` rows and the `PENDING→PUSHED→ACKED` state machine exist today only because policy is
prebuilt and pushed, so under on-demand pull they disappear for policy and survive **only** for
instant actions and terminal, where a durable command-delivery row is genuinely needed (see §7 for the
five ordinary-action push RPCs that must first be ruled on). The `EXPIRED`/`CANCELLED` delivery states
are **not** a removal mechanism — never written by production, never reaching the agent.

<!-- docref: begin src=server/internal/store/sqliteschema/schema.sql:d99cbd2d -->
- **Current schema is evidence, not a ruling.** The existing execution FK to `deliveries` shows
  how result correlation works today; it does not make the current `deliveries` table or its
  policy-push state machine non-negotiable. The target may retain, split, or replace it after
  the policy/run model is proven. Likewise, a canonical content fingerprint may be useful as a
  policy revision, but it is not by itself the lifecycle design.
<!-- docref: end -->

**Open problems that block implementation (must be designed + tested before Phase 5 deletion):**

- **O1 — Installed-policy identity is unsolved.** `ManifestProvenance` (`agent.proto:227`) carries
  only definition/action-set/action IDs; it does not identify the assignment, target path,
  compliance-policy source, ordering source, or window, and `assignments.sql:159` collapses distinct
  paths with `SELECT DISTINCT source_type, source_id, mode`. Design the minimal identity that is
  stable across sync/re-seal/runs and distinguishes everything that must reconcile independently.
- **O2 — Cross-sync result correlation is unsolved.** The compiler mints fresh `ulid.Make()` manifest
  and occurrence IDs every compile (`compiler.go:79,211,374`); an unchanged-revision device keeps its
  old IDs while the server re-resolves new ones. Choose deterministic/derived identifiers or a durable
  pulled-manifest→identity mapping.
- **O3 — The result protocol needs an explicit acceptance guarantee.** The current path requires an
  ACKed delivery (`result.go:72`), the first `ManifestResult` terminalizes the delivery
  (`state.go:274`), and the agent marks a result synced on socket-send success even if the server
  rejects it (`runtime.go:293`). Decide: add a correlated acceptance-ack, or explicitly accept silent
  loss. `run_id` and error-truncation do not settle this.
- **O4 — Removal safety + in-flight semantics.** A cascading DELETE can erase recovery state mid-run
  (`001_initial_schema.sql:14`). Removal must retire-then-defer; and a product decision is owed on
  whether unassignment cancels an executing run or lets it finish and report.
- **O5 — The five ordinary-action push RPCs (§7)** must each be ruled retire / convert-to-assign+trigger
  / preserve-as-external-one-shot before the policy-delivery state machine can be called removable.

Until O1–O5 have designs and passing acceptance tests, this model is a target *direction*; the
conditional architectural deletions in §5/§16 are not yet safe to schedule.

**The single biggest risk** is conflating the stable policy revision (must survive re-seal and
runs) with the fresh per-firing run id (must differ every firing) into one field — that
reintroduces either the churn bug or the replay-conflict bug. Keep them physically separate from
day one; write the two-recurring-runs test (different results, same revision, different run_id)
and the re-seal test (same revision, no churn) as the first two failing tests.

---

## 5. Ranked simplification opportunities

Ordered by confidence, not by line count. Only the first block is validated today.

1. **Validated private dead code (§6): 1,044 measured lines.** Safe after a build/test cycle.
2. **The mandatory floor fixes (§11).** Not deletions, but the highest-priority work.
3. **Merge duplicated decisions (§8/§12):** one `ResolvePlan`; `apierror` package; canonical
   instant-action classification at the contract level; `action-types.ts` from the descriptor;
   the FILE/DIRECTORY mode helper. Each is a bounded refactor or build-work, **not** a free cut.
4. **Conditional architecture deletions (unknown until Phase 4/5):** `DispatchAssignedActions` +
   `assignedManifests`; the policy push/delivery state machine + its ~1,257 test lines; the
   device-wide window union. Real, but gated on the model existing and its tests passing.
5. **Product-gated:** the five public RPCs; `respect_maintenance_window` (semantically clear now,
   proto-contract-gated); the write-only proto fields.

---

## 6. Confirmed private dead surface — 1,044 validated lines

Every item re-verified for (i) genuinely private (not exported SDK/public/proto surface),
(ii) **zero *production* callers** — several of these helpers are exercised only by tests that cover
the dead surface itself, and those tests are removed or edited with the code (per-item note in the
table); the two `auth/scope.go` helpers have zero callers including tests, (iii) build-safe. Measured
with `wc`; `deadcode` cross-checked; adversarially re-adjudicated.

| Item | Lines | Notes (re-verified) |
|---|---:|---|
| `fuzzyMaxHeap` + `fuzzyResultHeap` (`store/search_fuzzy.go`) | 44 | Both dead together; production uses `[]…` + `sort.Slice`. `container/heap` import dies with it. |
| `mtls.RequirePeerClass`/`RequirePeerClassNotRevoked` + helpers (`mtls/peer_class.go`) | 129 | `mtls` is server-internal (imported only by `agentstream`/`ca`/`enrollment`, not agent/sdk). Production gate is the inline `MTLSMiddleware`; **no coverage gap** — `identity_test.go` already asserts a revoked cert is refused 403. |
| `manifest.Compiler.Action/.ActionSet/.Definition` (`manifest/compiler.go`) | 13 | Production uses `*ForDevice` exclusively. Test must be *edited* to `*ForDevice`, not deleted. |
| `auth.EnforceSelfScope` (`auth/context.go`) | 20 | Production uses `EnforceUserScopeOrSelf`. |
| `auth.EnforceDeviceGroupScope` + `auth.IsDeviceScopeRestricted` (`auth/scope.go`) | 26 | Zero callers incl. tests. `IsDeviceScopeRestricted`'s doc comment claims a consumer that does not exist; the real anti-oracle fold (denial → NotFound) is elsewhere and tested. Remove the stale comment with it. |
| `mtls.DeviceIDFromTLS` (`mtls/mtls.go`) | 19 | Byte-identical dup of `DeviceIDFromRequest` (the production path). |
| `contract/ts/errors.ts` | 57 | `contract/package.json` is `"private": true`, so this is **not** external SDK surface; zero named importers; cites a deleted file. Remove the `export * from './errors'` in `index.ts` too. |
| Web UI `ui/calendar` (601) + `ui/collapsible` (47) + `ui/scroll-area` (84) | 732 | Zero references, barrel-aware; `range-calendar`/`date-range-picker` are distinct and live. |
| `executor.IsInstantAction` (agent) | 4 | The one instant-action copy nobody adopted; agent uses inline checks. |
| **Total** | **1,044** | + ~224 test lines removed/edited alongside. |

**Held OUT of the total (correctly FIX, not DELETE):** the three orphaned cleanup queries
(`DeleteExpiredRevocationsInTx`, `CleanupExpiredRevocations`, `DeleteTerminalJobsBefore`) — the
first two guard **unbounded-growth** tables (fleet×renewals; every session revocation), so they are
FIX-wire-in, not dead code; the third is bounded except under the C1 job bug. `store.IsAppendOnlyViolation`
is FIX — wire it into `PruneAuditPrefix`'s delete-error classification. The F-32 log-redaction
cluster (§11) is REVIEW.

---

## 7. Public API and SDK capability classifications

The whole point of this pass: internal reachability is not a lifecycle signal for reusable or
exposed surface.

- **SDK (`sdk/**`): KEEP all implemented capabilities**, including `sdk/sys/remote` Git/S3/HTTP,
  the seven "unreachable-from-Cadestro" packages, and their tests. The SDK is intentionally
  reusable, MIT-licensed mechanism code. Remove only for duplication, obsolescence, breakage, a
  harmful dependency, or an explicit product retirement.
- **Five public RPCs — REVIEW / PRODUCT DECISION, not dead:** `DispatchToGroup`,
  `GetDeviceCompliance`, `GetUserAssignments`, `ListAuditEvents`, `RenameToken`. Each has a
  handler, permission entry, generated client, and golden-contract presence. "No web caller" does
  not prove no external/future consumer.

<!-- docref: begin src=contract/ts/client.ts:2d78bd66,server/internal/identity/audit.go#Handlers.ListAuditEvents:5701e3cc -->
The shipped TypeScript client exposes these RPC capabilities, and `ListAuditEvents` has a
mounted server implementation. External or future use cannot be disproved by repository grep.
<!-- docref: end -->

- **`DispatchAssignedActions` — the one architecture-conditional RPC.** Its imperative semantics
  contradict the declarative model, and its only compiler helper has zero production callers.
  Delete only together with the `Sync→ResolvePlan` change and a deliberate contract ruling — not
  as a grep-delete.
- **Five ordinary-action push RPCs — each needs an explicit ruling (PRODUCT DECISION).**
  `DispatchAction`, `DispatchActionSet`, `DispatchDefinition`, `DispatchToMultiple`, and
  `DispatchToGroup` (`control.proto:2764-2769`, `dispatch/handlers.go:141`) compile **ordinary
  authored actions/sets/definitions** and push them as one-shot deliveries via `AsOneShot`
  (`handlers.go:541,556,572`) — not genuine instant actions like REBOOT/SYNC. This directly
  conflicts with the rule that only genuinely instant actions and terminal use server→agent push.
  **Correcting the earlier framing** (and §3's "instant = one-shot, 100% policy-agnostic"): the
  delivery layer is policy-agnostic, but these five RPCs push *policy content* through it. Each must
  be ruled: (a) **retire** it; (b) **convert** it to "create/change the assignment + trigger a sync"
  so the work flows through pull reconciliation; or (c) **preserve** it as a deliberate external
  one-shot exception. **Until all five are ruled, the policy-delivery state machine cannot be called
  removable** — Open problem O5.
- **`PrivilegeBackend` / `AdminPolicyParams.backend`: KEEP.** Required for the fail-closed doas
  handling (§11 S6). The first pass contradicted itself by proposing to delete it while requiring
  it; keep the selector and reject unimplemented doas execution.
- **Write-only proto fields — product-gated, never summed into any line total.** `default_on_failure`
  (documented as a provenance/audit field, never wired to a log — KEEP-as-provenance or delete),
  `Heartbeat.{cpu,memory,disk}_percent` (a half-built, never-wired telemetry feature — product
  call), `respect_maintenance_window` (inert — semantically safe to remove now, but a wire/golden
  change), `ActionResult.metadata` (deprecation-in-place; the "LPS password data" comment is stale
  and misleading; server hard-rejects any non-empty value), `OSQuery.where`/`LogQuery.source`
  (forward-declared query capability, SYSLOG has real intended value), `OSQuery.columns`
  (forwarded but the agent always does `SELECT *`). `SecurityAlert.message`/`.details` is **data-loss
  FIX**, not a delete (§11).

---

## 8. Redundant sources of truth and their canonical collapse

| Concept | Copies | Canonical target (ruling) |
|---|---|---|
| **Instant-action classification** {REBOOT, SYNC} | 5 (`dispatch/handlers.go`, `actionparams.isNoParamsActionType` — a superset incl. UNSPECIFIED, `executor.IsInstantAction`, two web) | **one** source, not two: encode "instant" on the proto `ActionType` itself (an enum-value option / the existing 500-599 numeric band) and **generate the predicate into both Go and TS** — a Go helper + a TS export would still be two hand-kept copies (**MERGE**) |
| **Error codes (3-way)** | 9 Go `errXxx` blocks + 2 packages inlining raw literals; `contract/ts/errors.ts` (dead, `"private"`); `web/src/lib/errors.ts` | Go values → codegen → TS + i18n with a parity test — but the generator **does not exist**, so this is FIX/build work, not a pure merge. Exact drift: 13 TS-only codes with no Go emitter, 23 Go codes with no i18n. Delete `contract/ts/errors.ts` now (§6). |
| **`rpcError` helper** | 10 packages; **not all identical** — the `identity` copy is a textually diverged rewrite (live drift evidence) | one server-only `server/internal/apierror` (**MERGE**, ~−70 lines); sequence before the error-code fix |
| **`action-types.ts`** | triple-enumerated (string→enum, enum→string, options) | derive from the generated protobuf-es descriptor (**SIMPLIFY**, ~109 lines); an existing round-trip+completeness test is the safety net |
| **Validators (contract weaker than executor)** | `DnfRepository.baseurl`/`PacmanRepository.server` (no URL check) + 4 more URL fields accept `http` where the SDK requires `https` | add validation at the contract boundary (**FIX**) — but this requires **writing and registering a new custom validator** (only `ulid` exists today), not adding a built-in tag; scope swept only the URL family |
| **Rate-limiter public-procedure list** | **4-way** (PublicProcedures map, `applyPublicLimiters` switch, `RateLimiters` struct fields, `defaultRateLimiters` ctor), no sync test | derive from one list with a completeness test (**FIX**). `PublicProcedures` itself is sound (2 tests). |
| **Audit-class parity guard** | 12 instances / 10 packages; 5 lack the completeness cross-check (`authoring`×3, `devicegroup`, `compliance`) | copy the existing `ElementsMatch` template into the 5 (**FIX**) |
| **Env vars** | server `configEnvironment` is self-discovering + fail-closed (sound) **except** `CADESTRO_DEV_AUTH`/`_TOKEN`, read raw → documented activation **crashes startup** (`os.Exit(2)`); `CADESTRO_CORS_ALLOW_ALL` works but is undocumented; the agent has **no registry** (fails open/silent on a typo) | fold the devauth vars into `configEnvironment`; mirror the pattern on the agent (**FIX**) |
| **Lifecycle states, backend `Detect()`** | one Go source each + SQL CHECK / one per SDK domain | **already sound** |

---

## 9. Weak / overcomplicated design

- **Two resolvers for one concept.** Preview and dispatch read identical source data but render
  differently — the confirmed sibling double-dispatch: two top-level ActionSets (one REQUIRED, one
  UNINSTALL) sharing an action emit conflicting PRESENT/ABSENT in one batch, because the
  `emittedActions` dedup seam is written in the definition/action_set loops but read only in the
  compliance loop. The target is **one `ResolvePlan`** — a bounded refactor: reuse the shared
  `ResolveSources` mode-precedence and the compiler's schedule/on_failure resolution; **add one
  deterministic, precedence-aware conflict resolution across assignment *sources*** (the single
  genuinely-new piece); delete the two `ErrEmptyManifest` abort branches. No strategy/factory. This
  also closes the preview/dispatch visibility-check divergence. **Two boundaries to keep distinct:**
  cross-source conflict resolution (two assignments both bringing action X) is *not* the same as the
  deliberate authored duplicates *within* a definition, which the proto explicitly promises execute
  separately — a naive "global dedup keyed on action id" would wrongly collapse the latter. And the
  `assignments.sort_order` column is a **dead column**, not a wiring opportunity: `CreateAssignmentRequest`
  has no such input and the upsert never writes it. Cross-source ordering is unspecified today; either
  add a real request input or **delete the column** — do not "wire" a field with no producer.
- **The redundant manifest push transport.** `manifest_delivery` is the one `ServerMessage`
  payload that duplicates what `SyncState` already refreshes; every other pushed payload is a
  genuine instant/terminal action. **SIMPLIFY to pull-with-instant-push** — but only after the
  reconciliation model (§4) carries the guarantees the push states provide.
- **Terminal ownership is diffuse** — one session lives in three packages with split close
  authority and a doubly-implemented `TerminalStop`; `ListActiveTerminalSessions` treats
  memory-vs-SQL drift as an internal error (proof there is no single owner). A focused
  consolidation is warranted, orthogonal to the policy work. Separate the genuinely-different
  lifetimes (ephemeral token / durable audit row / process routing) from the duplicated decisions.
- **Three staleness models.** `DeviceStatus` and `InventoryOverdue` are the same read-time
  stored-timestamp shape and could share a small helper *if it removes code*; `ComplianceStatus`
  is write-time materialized and should move to read-time derivation (§11 C2) rather than be
  folded in. Do not add a generalized staleness abstraction.

---

## 10. Correctness and reliability findings

Ordered by priority. Fixes, not simplifications.

- **[C1] Recurring maintenance jobs terminalize permanently — FIX, MEDIUM, confirmed.** The panic
  and error branches at `attempt_count >= max` call `finish(FAILED)` unconditionally; only the
  success branch re-arms. Correction to the first pass's "forever": within one process the job
  stops until the next restart re-seeds it. **Fix:** make the failure branches re-arm recurring
  kinds via the existing (already-audited) `Reschedule`, and alert on persistent failure. ~15-30
  lines, no schema. This is a symptom of over-forcing recurring work through a one-shot completion
  model — see §14 for why the durable runner nonetheless stays.

<!-- docref: begin src=server/internal/jobs/runner.go#Runner.Dispatch:a6caf02e,server/internal/maintenance/service.go#Service.EnsureScheduled:f580d969 -->
Successful recurring jobs are rescheduled, but a recurring job that reaches its attempt limit
is finished as failed; startup seeding only ensures the singleton exists. The smallest initial
fix is to define recurring failure/re-arm behavior and alerting inside this framework.
<!-- docref: end -->

- **[C2] Compliance grace clock is agent-controlled — FIX, MEDIUM (trust-relevant), confirmed.**
  `resultTime` takes the agent's `CompletedAt` verbatim with no skew clamp; a compromised or
  clock-skewed agent controls its own grace window, and a silent device freezes at its last status
  forever. **Fix:** clamp `checkedAt = min(agentCompletedAt, serverNow)` and derive status at read
  time (no sweep needed — §11). This belongs on the security floor, not just reliability.
- **[C3] Compliance verdict ignores the stored exit code — FIX, LOW, confirmed.** `detection_output`
  is stored but never read for the verdict; derive from `exit_code == 0 && success`. (Canonicalization
  / defense-in-depth — the device still supplies the code.)
- **[C4] One empty container aborts the whole batch — FIX, MEDIUM, confirmed.** `ErrEmptyManifest`
  poisons every other container; the target skips it. Zero tests either direction — add the edge test.
- **[C5] FILE/DIRECTORY mode-parse drift — MERGE (one helper), HIGH, confirmed by execution.** FILE
  uses `fmt.Sscanf("%o")` without masking; DIRECTORY was fixed (archived #174). Three empirically-
  verified defects: silent-skip on unparseable, partial parse (`"0777junk"`), and permanent re-apply
  of setuid modes. **Deeper:** the SDK's `validateMode` setuid/setgid rejection is *dead code* for
  any mode arriving through the `ParseUint→FileMode` cast (the cast never sets Go's `ModeSetuid` bit).
  Fix: one shared `parseUnixMode` helper used by both match and apply paths.
- **[C6] result_outbox — corrected framing, FIX, confirmed.** The agent send is fire-and-forget with
  no ack, so a *transport* failure retries forever (intended, keeps working while disconnected) while
  a server *rejection* is not observable — the row is marked synced and **silently dropped**. The
  first pass's "retried forever on rejection" is wrong. The real gap is the missing ack/reject signal;
  the smallest fix is to bound results at the boundary (truncate the un-truncated `result.Error` that
  can exceed the server's `max=4096`), not to add dead-letter machinery.
- **[C7] LUKS/LPS rotation interval 0 — FIX as defense-in-depth, MEDIUM, corrected.** In the executor,
  zero means "rotate every cycle" (LUKS churns keyslots; LPS rotates + reseals + kills the user's
  session every reconcile) because the due-check gate is an inverted `if RotationIntervalDays > 0`.
  **Correction to the first draft:** the claim that "the agent never re-validates" is wrong — the agent
  runs `validateInbound` on every `ManifestDelivery` (`contract/client.go:1419` → `validate.Struct`),
  and `rotation_interval_days = 0` is a validation-*rejected* value (pinned by
  `encryption_params_validation_test.go:82-88`, which asserts a zero interval must fail). So a
  well-formed delivery carrying zero is refused at ingestion, and the destructive executor branch is
  **not reachable through the normal validated path**. The residual is genuine defense-in-depth: the
  executor's inverted gate would misbehave if that validation were ever bypassed (a locally-constructed
  struct, a future non-`validateInbound` code path). Fix the gate to fail closed on zero. (One thing to
  confirm during implementation: that the delivery-level `dive` validation actually descends into the
  nested `EncryptionParams` oneof; either way the "never re-validates" framing was factually wrong.)
- **[C8] Config fragments written live, then validated — FIX, MEDIUM, confirmed.** `writeAndValidateConfig`
  writes to the final path then runs `visudo -c`/`sshd -t`. **Nuance:** the sudoers fix is a clean
  temp→validate→rename, but `sshd -t` validates the *actual on-disk merged config*, so a temp file
  outside the `.d` dir is invisible — the sshd side needs a synthetic Include-root or an accepted
  narrower window. The shared helper should carry a per-caller validation strategy.
- **Nav permission typo — FIX, confirmed.** `nav.ts` gates Tokens on `'ListRegistrationTokens'` and
  Terminal Sessions on `'ListTerminalSessions'`; the real permissions are `'ListTokens'` and
  `'ListActiveTerminalSessions'`, so both entries are permanently hidden from any non-admin holding
  the real grant. Add a parity test against the real registry.
- **osquery `crontab` preset — FIX (web), confirmed.** The web UI offers a `crontab` preset that the
  SDK's `sensitiveTables` denylist correctly refuses, so every click fails. Fix the **web** preset;
  **do not weaken the SDK denylist** — it is a KEEP security control (§14).
- **Also confirmed:** `RESTARTED` service state is the one non-idempotent branch (zero tests, single
  site, not auto-triggered) — if dropped, fail loudly; managed-block FILE has no BEGIN/END markers
  (duplicates on edit, silent no-op on drift, uncovered); `skip_if_unchanged`'s hash includes `Output`
  so it never suppresses varying stdout, and it suppresses the report not execution; scoped `ListUsers`
  reports a page count as the total; multi-facet search paging is incoherent and `searchFacets[:8]` is
  an untested positional gate; `device_groups.sync_interval_minutes` is write-only; `inventory_interval`
  is resolved server-side but the agent hardcodes 24h. Fabricated audit evidence: a typed-nil webhook
  notifier records a `NOTIFY_INTENT` for an impossible notification; `CleanupExpiredAuthStates` writes
  a hardcoded `remaining=0`; `EraseUser` understates its own cascade.

---

## 11. Security and audit-integrity findings (the floor)

Adjudicated by a deep-reasoning security pass; every item traced to a concrete defect path (zero
refutations of the retained set). **Blockers** are marked.

- **[S1] Privilege escalation to Admin — FIX, CRITICAL, RELEASE-BLOCKER, confirmed.** `checkGrantScope`
  runs `FirstPrivilegeGranting` only in its scoped branch; the unscoped branch waves through an
  "ordinary admin," and nothing compares the granted role's permissions against the actor's. A holder
  of only `AssignRoleToUser` can grant itself `AdminRoleID` unscoped and become global Admin (the
  self-grant even invalidates the actor's sessions, completing the exploit at re-auth).
  **`AssignRoleToUserGroup` shares `checkGrantScope`. `AddUserToGroup` is a *structurally distinct*
  exploit** — it never calls `checkGrantScope`, so adding oneself to an Admin-carrying group inherits
  Admin through a different path. **Fix = two enforcement points** (require conferred-authority ⊆
  actor-authority on the unscoped grant path *and* on the group-add path), **both** with a required
  carve-out for the bootstrap principal (whose 11 permissions are a strict subset of Admin, so a naive
  subset check would brick first-Admin setup). `RemoveUserFromGroup` is a different shape (lockout/DoS)
  — do not fold it in.
- **[S2] dnf/zypper trust unsigned packages by default — FIX, MEDIUM (downgraded from HIGH), confirmed.**
  Omitted `gpgcheck` renders `gpgcheck=0`. But the HTTPS-only base-URL is an *enforced* trust boundary
  and gpgcheck is a documented operator choice (ADR-0012) — this is a proto3 footgun / defense-in-depth
  gap, not an accidental hole. Fix: correct the comment and default the web form to true (already done);
  optionally reject signature-disabling at the boundary, matching pacman.
- **[S3] Authorization denials audited nowhere — ADD recorder, HIGH, confirmed.** `store.ResultFailure`
  has zero writers; `AuthzInterceptor` denies with no store handle. The exact fix pattern exists — the
  authentication-rejection path already uses a separate-tx, rate-limited `RecordOperation`. Give the
  authz interceptor the same recorder. A genuine ADD.
- **[S4] SSO redirect_url open-redirect → token exfiltration — ADD allow-list, HIGH, RELEASE-BLOCKER
  (for security-sensitive deploys), confirmed.** `redirect_url` is `required,url` only and flows to the
  OAuth `redirect_uri`; the public, unauthenticated `SSOCallback` returns the access+refresh tokens in
  its response body. Worse than a redirect — full account takeover, gated only on IdP redirect_uri
  looseness. Fix: an exact-match allow-list at `GetSSOLoginURL`; do not rely on the IdP.
- **[S6] doas silently executed as sudo — FIX (fail closed), MEDIUM, confirmed.** `executeSudo` ignores
  `params.Backend` and writes a sudoers file with a false success on a doas-only host. Fix: return
  unimplemented for DOAS. Keep the field.
- **[S7] Unlimited, never-expiring enrollment token by default — FIX, MEDIUM, confirmed.** The privileged
  `CreateToken` path takes proto defaults (`max_uses=0` = unlimited, `expires_at=nil` = never), and the
  token mints device mTLS. Bound them.
- **[S8] Identity zombie + last-link deletion — FIX (delete provisioning_source), MEDIUM-HIGH, confirmed.**
  `deleteUser` erases only when `remaining==0 && source==SCIM`, leaving an OIDC-JIT-origin user whose last
  link is a SCIM binding as a **zombie** (zero links, retained grants/email/linux_uid). The deep point:
  a zombie and a legitimate pre-provisioned invite are *structurally identical at bind time* (both 0-link
  + grants + email), and `mayBindByAddress` treats `bound==0` as "ordinary invite, bind freely" — so any
  provider, even `trust_email_assertions=false`, can re-bind to the zombie and inherit its privilege. No
  bind-time heuristic can separate the two without breaking the invite flow, so **the only robust fix is
  to never let zombies exist — erase on the last link, any source** (delete `provisioning_source`'s
  gating role). **Deleting `provisioning_source` is itself the security fix.** Required guards that must
  remain: the bind-time takeover guard (`linked>0 && !trust`) and the audited `trust_email_assertions`
  toggle. Interim (if erase can't ship immediately): at the *unbinding* boundary, strip grants/memberships
  and disable a non-erased 0-link account — a bind-time refuse is not acceptable.
- **[S9] FILE writes to `/etc/*.d` unvalidated — FIX, MEDIUM (re-scoped), confirmed.** The escalation
  framing is **refuted** (single `CreateAction` gate; FILE authorship is root-equivalent by design; the
  proposed `IsUnderProtectedPrefix` fix would break the feature). The residual is data-safety: FILE can
  write `/etc/sudoers.d/*`, `/etc/ssh/sshd_config.d/*`, `/etc/cron.d/*` with no `visudo`/`sshd -t` check,
  bricking sudo/sshd. Fix: syntax-validate writes into those specific drop-in dirs.
- **[S10] Audit prune/anchor not self-recorded — FIX, LOW-MEDIUM, confirmed (core sound).** `PruneAuditPrefix`
  — the one writer that destroys evidence — records no audit operation of itself. The cryptographic core
  holds (fail-closed prune requiring archive confirmation, append-only triggers, verification that fails
  closed). Emit an audit operation for prune and anchor.
- **[S11] SSH key stored raw — FIX (canonicalize), LOW, confirmed (not RCE).** Options survive into storage
  but the agent silently drops decorated keys — a broken feature, not RCE. Store `MarshalAuthorizedKey(parsed)`.
- **[C2] compliance grace clock** (§10) belongs here too as a compromised-agent trust gap.
- **SecurityAlert forensic drop — FIX (data-loss), MEDIUM, confirmed.** The rogue-server-reassignment alert
  populates `message` + `details[requested/registered_server]` (the forensic identity of the rogue server),
  and the handler reads only `alert.Type`, discarding both. Server-only fix: persist them.
- **[S5] Server module never race-tested — TEST-QUALITY gap, not a vulnerability, confirmed.** Add `-race`
  to `server.yml`/`verify.sh`; do not assign security severity until it finds a concrete race.
- **[F-32] Half-wired log-redaction subsystem — REVIEW, LOW, confirmed.** `sanitizeForLog` +
  `budgetedChunkCallback` are built and tested but never activated (`ExecuteWithStreaming` is always called
  with a nil callback; `OnManifestDeliveryWithStreaming` discards its callback). **The "live secret leak"
  framing is refuted** — the unsanitized `agent_update` log lines emit connectivity diagnostics and a fixed
  template, not secrets. Either wire the redaction end-to-end or remove the skeleton as a unit.

**Cross-cutting pattern (systemic):** proto3-bool-default-lie (insecure only at GPGCheck; footguns at the
provider bools and CreateToken; benign at Backend), validate-at-the-edge eroded to validate-at-the-executor
(invalid cron fails open to an 8-hour drift), and fail-open-in-the-untaken-branch. The codebase already
applies the fail-closed `oneof=` idiom to two enums and pacman's SigLevel — extend it.

**Identity lifecycle threat verdict:** deleting `provisioning_source` (last-link-governs) is **safe** and
is the fix, with the bind-time guard and audited toggle retained; the current zombie re-bind vector needs
the interim strip-at-unbind patch if the erase change can't ship at once.

---

## 12. General code-quality and test-quality findings

- **`rpcError` + error-code constants duplicated across 10 packages (one diverged) — MERGE** into a shared
  `server/internal/apierror` package; sequence before the error-code canonicalization.
- **Two ~90%-identical list-state factories** share ~150 lines. **Honest verdict: extract only the tiny pure
  `readFilters` helper.** Folding the getter/method block needs a row-strategy parameter — an abstraction the
  brief warns against without a measured net deletion or a third behavior; with two behaviors and an
  unmeasured saving, copying identical one-line getters is simpler than a strategy interface.
- **Accessibility: 5 forms have zero label/input association** (AdminPolicy, Encryption, LPS, Sshd, WiFi +
  `UserPicker`) — the highest-stakes credential forms. Systematic, mechanical FIX; must not be simplified away.
- **Test quality:** SDK tests protect intentionally-preserved capability and are **not** dead. Genuine gaps:
  `EnforceSelfScope` has a test for the unused copy; the audit-class parity guard is missing its completeness
  cross-check in 5 of 10 packages; `ErrEmptyManifest` and `selectedFacets[:8]` and the public-procedure-limiter
  invariant are untested; `web/tests/showcase` is a zero-assertion screenshot suite backed by a 1,399-line
  hand-maintained permission fixture that already drifted once. Strong surfaces to keep (calibration):
  `store/api_shape_test.go`'s reflection+completeness guard, the credential matrix through the real interceptor,
  the descriptor-derived mount test, and the `security_machine_test.go` "zero calls reached the escalated
  runner" assertions.
- **Tests coupled to the redesign** (invert/delete as one unit *after* the model lands): the
  `DispatchAssignedActions` tests, the delivery push/receipt state-machine tests (~1,257 lines), the
  `provisioning_source`-gated erasure tests, the device-wide window-union tests.

---

## 13. Documentation, deployment and operational findings

- **Launch blocker (P0):** README/`docs/installation.md` fetch `releases/latest/download/install.sh` to
  install *control*, but the release workflow publishes the **agent** installer under that name; the deploy
  tree is never a release asset. Couples to publishing a signed deploy asset.
- **Release least-privilege:** `release.yml` grants release-grade write to every test/build job at the top
  level; `web-image` narrows correctly. **Corrected:** `agent-integration` is **not** elevated — a reusable
  workflow caps at its own declared `contents: read` regardless of the caller.
- **`-race`** is missing from the server lane only — a **test-quality** gap (§11 S5).
- **`PRAGMA user_version == 1`** is hardcoded in 6 operational sites (+ a 7th in a test), one of them a dead,
  superseded top-of-schema statement. Delete the dead line; share the literal.
- **Container root:** `Containerfile.control` runs as root while web drops to `USER bun`; a rootless cutover
  is a coordinated image+volume-ownership+smoke change.
- **`IMAGE_TAG=latest`** sits under its own "pin in production"; the branch-install fallback fetches unsigned
  source; the `MANCHTOOLS` uppercase repo default is a likely-harmless case inconsistency.
- **Ten stale enrollment-socket texts** ("no sudo / any user / 0666") contradict the actual 0600 + `SO_PEERCRED`
  fail-closed code (two print at install time; the proto ships the inverted claim to third-party authors). The
  LUKS-socket "no sudo" text (a *different* 0622 socket) is correctly excluded.
- **Seven doc/code drift items** confirmed with both sides: the never-built `default_on_failure` log promise;
  `--token` vs `--token-file`; the stale `metadata` example; bootstrap "minimal" vs 11 perms; the
  maintenance-window doc anchored to `Union` describing the *opposite* of production behavior (docref verifies
  the symbol, not the caller-decided semantics); the "three containers behind Traefik" double-count; the
  dangling `../DESIGN_2026_07_31/` reference.
- **Backup health:** stale-backup alerting **already exists** via the `EventBackupLag` webhook; wiring it into
  `/ready` would (rightly) fail the first install. The owner ask is already served.
- **Upgrade:** no migration machinery (reinstall-clean); the four evidence tables' append-only triggers permit
  `ALTER TABLE ADD COLUMN` but forbid row rewrites — the minimal manual-upgrade shape is single-transaction
  additive `upgrades/NNNN_to_NNNN.sql`.
- **CI hygiene:** the lint sequence is copy-pasted (and already drifted) across four workflows with unpinned
  `@latest` tool installs — a composite action + pinned versions.

---

## 14. Complex-looking parts that are justified (KEEP)

Calibration — do not delete necessary complexity to raise the count.

- **The durable job runner: KEEP-and-FIX** (only the C1 re-arm). Confirmed first-hand: `WithAudit` defaults
  to the `control` stream that `VerifyAuditChain` walks; the runner's own Claim/Finish/Reschedule rows are the
  **only** tamper-evident proof that the audit-verify/anchor/retention jobs fired (those jobs write no chain
  evidence of their own firing). A `time.Ticker` replacement either loses that proof or rebuilds `state.go`
  under another name — the simplification evaporates. (Multi-instance safety is a genuine open product decision,
  not a settled "single-instance forever" — §15.)
- **Crash recovery** (`RecoverInterruptedOccurrences`, reboot-boot-id) is a load-bearing at-most-once guarantee.
- **The audit hash chain and its fail-closed verification;** the cryptographic core (ECIES sealing, CA-pin,
  signed self-update, LUKS add→store→verify→remove ordering, LPS report-before-set); the SDK anti-injection
  validators and the **osquery `sensitiveTables` denylist** (a self-discovering, tested KEEP security control —
  the drift is the web preset, not the denylist).
- **The bind-time account-takeover guard** and the audited `trust_email_assertions` toggle — the identity
  simplification leaves them intact.
- **Implemented SDK capability with no in-repo consumer** — reusable product surface; preserve.
- **Single-implementation interfaces are overwhelmingly deliberate** (least-privilege `*store.Store` handles,
  DI test seams, SDK plugin contracts); only `osquery.Querier` (self-admittedly) and the 5 Backend-enum
  Managers are candidates, and 3 of those 5 are actually test-substituted.
- **The deploy shell layer** (`umask 077`, env parsed-not-sourced, three-layer archive isolation, the OpenSSL
  `-checkhost` workaround, the docker.sock refusal and query-string credential canary).
- `actionparams`' reflection registry, `dynamicquery`, the terminal three-tier lifetime model, `testdb`'s
  pgx shim.

---

## 15. Product decisions and unresolved questions

1. **Ordered-definition STOP scope.** Today a Definition compiles to one manifest per set, so STOP does not
   cross sets. Merging into one ordered manifest — needed to honor `sort_order` — would make a set-1 STOP
   silently halt set 2 (the flat loop + single `stop` flag inherit this with no code change). Get an explicit
   ruling before merging.
2. **The five REVIEW-candidate public RPCs' lifecycle** (`DispatchToGroup`, `GetDeviceCompliance`,
   `GetUserAssignments`, `ListAuditEvents`, `RenameToken`) — supported API or retired? Decide per-RPC, not by grep.
2b. **The five *ordinary-action push* RPCs (O5, §7)** (`DispatchAction`, `DispatchActionSet`, `DispatchDefinition`,
   `DispatchToMultiple`, `DispatchToGroup`) — each: retire / convert to assign+sync-trigger / preserve as a
   deliberate external one-shot exception. The policy-delivery state machine is not removable until this is ruled.
3. **Policy / revision / run identity shape (O1–O3)** — the installed-policy identity, cross-sync result
   correlation, and the result-acceptance guarantee (correlated ack vs. accepted silent loss) — decide together
   with the reconciliation model (§4). None is solved yet.
3b. **Unassignment vs an in-flight run (O4)** — does removing an assignment cancel an executing occurrence or
   let it finish and report? Removal must be retire-then-defer regardless, to avoid erasing recovery state mid-run.
4. **Current vs historical execution/compliance** retention and read semantics.
5. **Job-runner multi-instance future** — the swing factor for whether the leases/dedupe simplify or are
   load-bearing. Unexercised today ≠ unneeded.
6. **`respect_maintenance_window`** — semantically clear for removal now (zero readers), but a wire/golden change.
7. **Per-occurrence multi-group maintenance windows** — an occurrence reachable via two windowed groups must
   union *those* groups' windows (a scoped `Union`, deleting only the device-wide caller).
8. **Monthly windows** — the weekly-only model; per-action monthly cadence is already served by `ActionSchedule.cron`
   (5-field, incl. day-of-month); only a monthly-restricted *dispatch gate* is a genuine ADD.
9. **`PackageParams.version` across mixed fleets** — decide from real authored-action data before touching.
10. **F-32** — wire the streaming-redaction subsystem or remove it as a unit.
11. **Reference-deployment scale** — determines whether re-resolve+re-seal every sync is cheap (a bound, not
    a measured fit).

---

## 16. Evidence-first phased plan

The ordering prevents deletion from outrunning the missing model.

**Phase 0 — specify + write failing-first tests (before any architectural deletion).** Acceptance tests for
ADD / CHANGE (re-seal = no-change) / REMOVE / remove-all / group add-remove / first-unchanged-restart /
interrupted-reconcile / two recurring runs with different results / replay-vs-conflict per run_id / current-vs-history.
These must fail on `99ab489` (removal and second-run are unrepresentable today).

**Phase 1 — the mandatory floor (§11), highest priority.** S1 (two fix points + bootstrap carve-out) first,
then S4, S3, C1 re-arm/alert, C2 clamp + read-time compliance, S8 last-link erase (+ interim strip-at-unbind),
S6, S7, S9, S10, C5/C7/C8, SecurityAlert persist; add `-race`; fix C6 at the result boundary.

**Phase 2 — validated private cuts (§6), ~1,044 lines.** Delete only the confirmed-private, zero-caller,
build-safe surface; edit the coupled tests. Re-measure after each class.

**Phase 3 — merge duplicated decisions.** One `apierror` package, then error-code codegen (build work); the
one `ResolvePlan` (prove preview parity first); canonical instant-action at the contract level; `action-types.ts`
from the descriptor; the mode helper; the `readFilters` extraction. No speculative factories.

**Phase 4 — design + build + test the reconciliation/run model (§4). This phase must first close the
Open problems O1–O5, not assume them.** On-demand `ResolvePlan` (nothing prebuilt or materialized on
change); **design the stable installed-policy identity (O1)** — provenance is only a candidate part;
**solve cross-sync result correlation (O2)** with derived/deterministic identifiers or a durable
pulled-manifest mapping (fresh `ulid.Make()` per compile is the blocker); **decide the result-acceptance
guarantee (O3)** — correlated ack vs. explicitly-accepted silent loss, and remove the ACKed-delivery
precondition + delivery-terminalization coupling; **make removal retire-then-defer and rule in-flight
cancellation (O4)**; **rule the five ordinary-action push RPCs (O5, §7)**. Pre-seal revision, `run_id` +
run-history evidence table, results recorded on report, wire `Sync→ResolvePlan`, prove preview parity.

**Phase 5 — conditional architectural deletion (only after Phase 4's O1–O5 designs and their acceptance
tests are green).** `DispatchAssignedActions` + `assignedManifests`; the policy push/delivery states +
~1,257 test lines (only for whichever `Dispatch*` RPCs O5 retires/converts); ordered-definition merge
(after the STOP ruling); per-assignment windows then delete the device-wide union; repoint/collapse the
`deliveries` FK last.

**Phase 6 — optional product additions.** Monthly-window gate, flatpak-remote authoring, retention classifier,
SQL upgrade files. Never remove implemented SDK capability as part of this audit.

---

## 17. Compatibility and migration consequences

- **Pre-alpha permits clean breaks;** but public-RPC and proto-field removal still needs a product ruling plus
  a generated-client/golden-contract update. `rpc_golden.json` records surface, not deadness.
- **The SDK is intentionally external-facing;** repository-local import counts are irrelevant to its compatibility.
- **Schema changes** touch the `PRAGMA user_version` literal in 6 sites; the four evidence tables' append-only
  triggers permit `ALTER TABLE ADD COLUMN` (e.g., `run_id`) but forbid row rewrites.
- **Sync reconciliation changes observable behavior and stored identity** — a device that received nothing will
  begin receiving its assignments; recurring results stop being silently dropped; removed policy actually stops.
  Rollout needs the full add/change/remove/restart/interrupted/two-run test set, not a two-sync idempotency check.
- **Identity:** the `provisioning_source` change inverts two pinned tests, needs a schema migration, and the
  erasure is irreversible (crypto-shred of the subject DEK) — the provider form must say so.

---

## 18. Measured reduction estimate

| Bucket | Lines | Status |
|---|---:|---|
| Validated private dead code (§6) | **1,044** (+ ~224 test) | measured, safe now |
| Merge/simplify (apierror ~70, action-types ~109, resolver dedup, mode helper) | order of a few hundred | net-negative but **build work**, not free cuts; error-code codegen does not exist yet |
| Conditional architecture (assigned-dispatch, policy push states + ~1,257 test lines, window union) | ~2,500+ | **unknown until** the Phase-4 model + tests exist |
| Public RPCs, `respect_maintenance_window`, write-only proto fields | — | **unknown until** product ruling |

**Defensible net today: the 1,044 private lines** plus the floor fixes (which add, not remove). Everything larger
is a *consequence* of building the reconciliation model and passing its tests — not an independent, bankable
deletion. The success metric is fewer concepts and one decision path, not a maximized line count.

---

## 19. Coverage and model ledger

Two waves. **Pass 1:** 15 Sonnet discovery agents + 2 Opus verifiers. **Pass 2 (this revision):** 12 Sonnet
deep agents + 3 Opus deep tasks. **Zero Fable subagents across both.** The Fable orchestrator coordinated and
re-verified the headline claims first-hand.

| Pass-2 agent | Model | Scope |
|---|---|---|
| RA | sonnet | agent manifest store / occurrence lifecycle / removal gap / 2nd-run / crash recovery / outbox / secret churn |
| RS2 | sonnet | server sync / execution identity / delivery / result history / FK |
| RES | sonnet | the two resolvers / precedence / sibling double-dispatch / unified ResolvePlan / order |
| JOB | sonnet | durable job-runner durability comparison / smallest fix |
| SEC | sonnet | adversarial re-verify of the security floor S1–S11 |
| CMP | sonnet | server-derived compliance read-time vs sweep / per-assignment windows |
| SOT | sonnet | sources-of-truth + validator/capability/permissions/env sweep |
| DPRIV | sonnet | strict private dead surface, build-aware, measured |
| PF2 | sonnet | proto fields per-field producer/consumer |
| EXE | sonnet | executor semantics / STOP scope / mode-parse / rotation |
| WEB | sonnet | web quality / forms / a11y / dead UI / deps |
| OPS | sonnet | deploy / CI / release topology / doc drift |
| OPUS-ARCH | opus | 15-scenario lifecycle trace + reconciliation/run model + data model + sequenced plan |
| OPUS-SEC | opus | floor adjudication + identity threat model + job durability + compliance |
| OPUS-ADJ | opus | final adversarial adjudication of deletions + preservation compliance + completion gate |

**Directories covered:** all of `agent/`, handwritten `contract/`, `sdk/`, `server/`, `web/`, `docs/`, `.github/`,
`server/deploy/`, root prose. Generated code was compared to canonical sources, not audited as handwritten.

**Completion-gate checklist (all confirmed):** assignment-removal traced to a concrete answer (no path at any
layer; fix = provenance `ReconcilePlan` set-difference DELETE); two recurring runs traced (run 2 unrepresentable,
silently dropped; fix = `run_id` + `execution_runs`); SDK/public-API preservation rules followed (PASS on all six);
the line estimate limited to validated cuts; no first-pass error recurred.

**Honest coverage gaps:** the validator-drift sweep covered only the URL field family, not all ~26 SDK domains;
the agent executor's cross-occurrence STOP semantics under a *hypothetical* merged manifest were reasoned, not
tested; the reference-deployment scale and the job-runner multi-instance intent cannot be settled read-only; the
zombie re-bind vector is a traced code path, not an integration-proven exploit.

---

## 20. Rejected or refuted candidate findings, and provenance

**Refuted / corrected (so they are not rediscovered):**

- **SDK packages / `sdk/sys/remote` are dead because Cadestro doesn't import them — REFUTED.** Reusable product
  surface; preserved.
- **The five public RPCs are dead because the web UI doesn't call them — REFUTED.** Internal caller count is not
  external-contract intent. `DispatchAssignedActions` is a separate architecture decision.
- **Content-hash + insert-if-absent completes pull reconciliation — REFUTED.** It has no delete and churns on
  re-seal.
- **Six process-local tickers are automatically simpler than the durable job runner — REFUTED.** The runner's
  audited transitions are the only tamper-evident proof the audit jobs fired.
- **The `deliveries` table is non-negotiable — REFUTED.** Evidence of the current impl, not a product invariant.
- **A recurring compliance sweep is mandatory — REFUTED.** Read-time derivation is sufficient for the two identity
  consumers; only a durable transition row, a webhook-on-transition, or an indexed sort-at-scale would force one.
- **`@connectrpc/connect-web`, `idb`, `superjson` are removable web deps — REFUTED.** All three are load-bearing
  through the `$contractClient = '../contract/ts'` alias (a package.json-less sibling); only `postcss` is removable.
  The "grep web/src for the name" heuristic is unsound for this layout.
- **FILE-action root-config injection is a *privilege escalation* — REFUTED.** Single `CreateAction` gate; the
  residual is a data-safety validation gap (S9).
- **F-32 is a live secret-in-logs exposure — REFUTED.** The dead-control observation stands (REVIEW); the logged
  output is connectivity diagnostics and a fixed template, not secrets.
- **The osquery `crontab` denylist lives in `server/` and should be relaxed — REFUTED / RELOCATED.** It is in
  `sdk/sys/osquery`, a KEEP security control; the defect is the web preset offering a table the SDK refuses.
- **The compliance verdict "trust boundary is crossed" — partially overstated.** Deriving from `exit_code` still
  trusts the device-supplied code; it is canonicalization, not trust elimination.
- **`agent-integration` job over-privileged — corrected.** The reusable workflow caps at its own `contents: read`.
- **Two form-validation items** (agent-update `http://`, empty sshd directive) — already fixed in this tree.
- **Seed MISREADs re-confirmed:** crypto-shred exists (only the term is unnamed); the enrollment socket is
  fail-closed (the texts are stale); the CA pin is cert DER; the compliance status enum never crosses the wire.

**Provenance.**
- **Tree:** commit `99ab489`. **Dirty worktree preserved** (the untracked seed docs and this file only); no tracked
  file modified.
- **Read-only throughout.** Non-mutating validation actually run this pass: `go build`/`go vet` clean baselines,
  targeted `go test` on scheduler/store paths (agent reconciliation and skip-if-unchanged tests executed live), a
  `go run` verifying `os.FileMode` setuid-bit semantics, `deadcode` cross-checks, and orchestrator first-hand
  re-verification of the headline claims — `contract/package.json` is `"private": true`; the agent has no
  `DELETE FROM manifest_deliveries` and the `ServerMessage` oneof carries no removal payload; `respect_maintenance_window`
  has zero non-generated readers; the `AssignRoleToUser` privilege check runs only on the scoped branch; the seven
  SDK packages total 3,750 impl + 6,264 test lines with zero non-test importers (KEEP, not delete).
- **`docref check SIMPLIFICATION-AUDIT.md` passes** (the six behavioral claims above resolve against unchanged source).
- **Verifier coverage:** every high-impact deletion and architecture claim was sent to an independent Opus verifier
  instructed to refute it; the final adjudication re-verified the 1,044-line deletion list per item and the
  preservation-rule compliance. Findings that could not survive verification are above.
- **Model discipline:** pass 1 = 15 Sonnet + 2 Opus; pass 2 = 12 Sonnet + 3 Opus; **0 Fable subagents** in either.

The desired outcome is still the smallest trustworthy Cadestro: **build the missing desired-state reconciliation
and recurring-run model and prove it with failing-first tests; harden the security floor, led by the privilege
escalation; delete only the 1,044 lines of genuinely private dead code now, and the larger architecture only as a
tested consequence of the model; and preserve every intentional SDK and public-API capability.**
