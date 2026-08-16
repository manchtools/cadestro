# Wave 2 — what the same care finds in the areas round one did not reach

Round one answered your audit questions and swept the contract and docs. Wave
two reads the code those questions did not touch: server internals beyond
dispatch/identity, the agent executors and SDK backends, and web/deploy/CI.

Read-only throughout — no builds, no tests, no processes. Every claim carries
a file:line. Findings are severity-ordered within each section; each section
ends with what was checked and found **sound**, because a report without
calibration is just a list of complaints.

Status: **complete** — five independent scans (A–E). Where two readers reached
the same conclusion independently it is marked as confirmed; where they
disagreed, the disagreement is reconciled in place rather than averaged away.

---

## A. Server RPC surface, authorization, and audit integrity

**The surface, verified rather than assumed:** `controlrpc.Handlers.Mount`
(`server/internal/controlrpc/mount.go:38-58`) mounts **158 procedures —
exactly the `ControlService` descriptor set**, no duplicates, pinned by
`mount_test.go:25-68`. Audit classification covers the same 158: **108
mutation / 37 read / 13 sensitive read**. Gating: 7 public
(`auth/interceptor.go:39-47`), 5 alternatives-gated (`:65-89`), 146
name-derived permission via `ProcedureAction` → `Authorize` (`:605-612`).

One fact underpins several findings below: a *scoped* grant still places the
base permission in the flat set (`auth/scope.go:236-238`), so the interceptor
admits scope-confined callers unconditionally and **each handler alone does
the confinement**. That is a deliberate design, but it means a handler that
forgets is not caught anywhere else.

### A1 — An authenticated caller can probe the entire permission surface without leaving a trace
**SECURITY / AUDIT-INTEGRITY GAP.** Every domain's `operation()` helper
hardcodes `AuthorizationAllowed` + success (`searchrpc/handlers.go:130-131`,
`device/handlers.go:216-218`, `authoring/handlers.go:93`,
`dispatch/handlers.go:124`, `devicegroup/handlers.go:110`,
`compliance/handlers.go:105`, `assignment/handlers.go:78`,
`registrationtoken/handlers.go:100`). `AuthzInterceptor` denies **with no
store handle at all** (`auth/interceptor.go:594-613`), so a denial cannot be
recorded even in principle. `store.ResultFailure` (`store/audit.go:54`) has
**zero writers repo-wide**. Failed *authentication* is audited
(`auth/rejection_audit.go:40-51`); failed *authorization* is not.

Weigh this against the product's own claim — "every mutation, sensitive read,
rejected authentication attempt, and background writer has structurally
enforced audit coverage". Rejected *authorization* is absent from that list
and absent from the code. For a product whose thesis is evidence, an insider
enumerating what they can reach is exactly the event an auditor asks about.

### A2 — The audit-log read path the UI uses is the one that is not audited
**AUDIT-INTEGRITY GAP.** Three ways to read audit evidence:
`ExportAuditEvents` (`identity/audit.go:176-183`) and `Search(scope=AUDIT_EVENTS)`
(`searchrpc/handlers.go:232-242`) both record `ClassSensitiveRead`.
`ListAuditEvents` (`identity/audit.go:66-120`) is classified an ordinary Read
and writes no evidence — and it is the paging path the UI uses. All three are
gated by the same permission (Export is registered as an *alternative* of
`ListAuditEvents`, `interceptor.go:86-88`), so the distinction is not
authority, only bookkeeping. Round one recorded "ListAuditEvents is not itself
a sensitive read" as a documentation nuance; seeing the three paths together,
it reads as an oversight rather than a decision.

### A3 — Global search is gated by a positional slice index, with no test
**LATENT SECURITY BUG.** `selectedFacets` returns `searchFacets[:8]`
(`searchrpc/handlers.go:271-281`). Excluding `executions` (index 8) and
`audit_events` (index 9) from unscoped search is **positional only** — insert
or reorder a facet and unscoped search silently widens. The compounding
detail: the sensitive-read audit fires only when
`req.Msg.Scope == SEARCH_SCOPE_AUDIT_EVENTS` (`:232`), so audit rows reaching
a caller through a widened *unscoped* search would be an **unaudited**
sensitive read. `selectedFacets`, `searchFacets` and `SEARCH_SCOPE_UNSPECIFIED`
appear in **no test file**. This is the "self-discovering guard" rule inverted:
a hardcoded index where a named exclusion set belongs.

### A4 — The bootstrap principal bypasses every rate limiter
**SECURITY GAP, bounded.** `interceptor.go:407-409` routes the
`Cadestro-Bootstrap` scheme to `authenticateBootstrap`, which calls `next()`
(`:470`) without consulting `limiters.Authenticated` or `limiters.Expensive` —
those exist only in the Bearer branch (`:430-439`). Authority is correctly
narrow (`identity/bootstrap.go:243-257`) and bad tokens are throttled by
`Rejected`, so the blast radius is small — but Search, export and dynamic-query
evaluation have no ceiling for that principal. Related: round one found the
bootstrap authority list is **11** permissions including
`UpdateIdentityProvider`, while the security doc describes it as minimal.

### A5 — A scope-restricted `ListUsers` reports a page count as the total
**CORRECTNESS BUG, user-visible.** `identity/users.go:161-165` sets
`TotalCount = len(resp.Users)` for confined callers — the rows that survived
an in-Go filter of *one page* (`:133-150`), not the number of users they can
see. Pages also under-fill, because the store pages *before* the scope filter,
and `NextPageToken` derives from the raw rows (`:151-153`). A scoped admin sees
a wrong total and short pages.

### A6 — Self-scoping has two deciders; the tested one is dead
**DEAD CODE + TEST-QUALITY DEFECT.** `auth.EnforceSelfScope`
(`auth/context.go:101-113`) has no production caller — its only callers are
`auth/authorizer_test.go:125-146`. Production self-scoping runs through
`EnforceUserScopeOrSelf` (`auth/scope.go:242-272`) from `identity/users.go:60`.
So the test that appears to prove self-scoping exercises the copy that is not
wired in. Exactly the shape the test-quality rules warn about.

### A7 — Search visibility SQL drops a soft-delete predicate its siblings carry
**DEFENSE-IN-DEPTH DIVERGENCE (not currently exploitable).**
`store/search.go:412-414` (devices) and `:425-427` (executions) join
`device_assigned_groups → user_group_members` **without**
`user_groups.is_deleted = FALSE`, while `queries/devices.sql:56`, `:104` and
`IsDeviceAssignedToUser` (`:198-205`) all carry it. Safe today only because
`DeleteUserGroup` hard-deletes memberships in the same transaction
(`identity/groups.go:275-277`) — i.e. the search predicate is correct by
accident of another function's implementation. The `device_assigned_groups`
row itself is never cleaned up.

### A8 — Multi-facet search paging is incoherent
**CORRECTNESS BUG.** `searchrpc/handlers.go:224-246` applies one `offset` to
every selected facet, returns one `next_page_token` for all, and sets `more`
if *any* facet has more. Page 2 of a global search therefore skips `page_size`
rows **inside each facet independently** — results are silently dropped.

### A9 — The audit-class parity guard is missing in 5 of 10 packages
**GUARD-COVERAGE GAP.** Present for device, identity, registrationtoken,
enrollment, dispatch, assignment, search. Absent for authoring ×3
(`store/action_handlers_test.go:429-440`), devicegroup
(`store/device_group_handlers_test.go:247-268`) and compliance
(`store/compliance_policy_handlers_test.go:388-400`) — those assert Mount
against a **hardcoded list** and separately iterate `MutationProcedures()`
only. The lists are complete today; nothing holds them there. This is the
"self-discovering guard with a matches-zero assertion" rule applied in half
the codebase.

### A10 — Two more instances of wave-one patterns
- **Validate-at-the-edge erosion:** `SearchRequest.date_filters`
  (`contract/proto/cadestro/v1/control.proto:2575`) carries no validator and
  no `dive`, so `SearchDateFilter.field/start/end` reach the handler
  unvalidated (`searchrpc/handlers.go:172-178`); the allow-list is store-side
  (`store/search.go:169-174`). Fails closed, but the boundary is not the
  boundary.
- **Two hand-maintained lists that must agree:** `PublicProcedures`
  (`interceptor.go:39-47`) and the limiter switch (`:475-499`).
  `permissions_test.go:139-149` asserts each public procedure names a real
  RPC; **nothing asserts each has a limiter**, so a new public procedure ships
  unthrottled.

Corroborated from wave one: `connect.WithRequireConnectProtocolHeader()` is
still absent from the handler options (`controlruntime/runtime.go:132-141`) —
the cheap CSRF primitive for the HttpOnly-cookie discussion.

### Where the floor holds (section A calibration)

Mounted set equals the descriptor set with duplicate detection; permission↔RPC
parity is asserted **in both directions** with non-stale exemption guards
(`permissions_test.go:83-137`); the alternatives map is size-pinned and
snapshot-copied against mutation (`:151-196`); the expensive-procedure set is
descriptor-backed and exact (`:225-248`); streaming is refused at both
interceptors; **validation runs before authentication**, matching the design's
stated order (`runtime.go:133-139`); search sort/tag/date names are
allow-listed maps never interpolated into SQL, with `escapeLikePrefix`
covering LIKE metacharacters (`store/search.go:226-228`) — **no injection
surface in the search path**; trusted-proxy XFF resolution walks right-to-left
and is well tested (`interceptor_test.go:20-78`); and the `:assigned` tier's
SQL (`queries/devices.sql:185-206`) correctly covers both direct and
user-group assignment, matching the search predicate — a suspected divergence
there did **not** survive checking.

---

## B. Server internals: audit chain, retention, background writers, archive

This section reads the machinery the product's thesis rests on. The headline
is uncomfortable and worth stating first: **the evidence system's own
integrity operations are the least observable part of the system.**

### B1 — The one writer that destroys evidence is the one that records nothing
**HIGH.** `PruneAuditPrefix` (`store/audit_chain.go:482-591`) deletes audit
rows and records **no audit operation of its own**. Anchoring
(`audit_chain.go:350-394`) and verification are likewise silent. Every other
background writer goes through `WithAudit` (`store/maintenance.go:26`,
`jobs/state.go:145,213,273,313`, `maintenance/service.go:407,446`). The
checkpoint row it does write is bookkeeping, not evidence: the
`audit_event_rows` view (`schema.sql:804-875`) reads only operations and
effects, so **retention never appears in List or Export**. Against the stated
property — "every mutation, sensitive read, rejected authentication attempt,
and background writer has structurally enforced audit coverage" — the
deletion of evidence is the uncovered case.

### B2 — A failed audit-chain verification fails silently, then stops forever
**HIGH.** `maintenance/service.go:163-208` with `jobs/runner.go:178,233-242`:
a verification failure surfaces as `logger.Error("job dispatch failed",
code=RUNNER_ERROR)` **with the error value deliberately dropped**. After
`maintenanceMaxAttempts = 100` (`service.go:40`) the job goes FAILED and is
never rescheduled — rescheduling happens only on success — and
`EnsureScheduled` (`service.go:130`) skips only PENDING/CLAIMED, so absent a
process restart **verification stops permanently**. The webhook registry
(`webhook/client.go:97-104`) has no audit-integrity event, so nothing alerts.
A tampered chain and a broken verifier look identical from outside.

### B3 — Retention has no table classification and no fail-closed default
**HIGH, and it contradicts a recorded architectural ruling.** The only
retention policy is `CADESTRO_AUDIT_RETENTION` on the chain
(`config.go:62,285`). `device_inventory`, `osquery_results`,
`log_query_results`, `terminal_sessions`, `security_alerts` and `executions`
hold hostnames, interface addresses, usernames and command output with **no
retention concept and no place where a new table is forced to declare one**
("executions are never pruned" is stated as fact in the code). The mono-era
ruling R4 required the opposite: *"the retention job refuses to delete from a
table that is not explicitly classified, so a new table is exempt by default
until someone classifies it. Fail closed."* The fail-closed classifier was
never built. For a product selling regulated-environment auditability, "we
keep personal data in six tables with no retention policy" is a question you
will be asked.

### B4 — Three retention sweeps are implemented, documented, and never called
**HIGH.** `queries/revoked_certificates.sql:33-45`, `revoked_tokens.sql:8-9`,
`jobs.sql:95-99` — **zero Go callers**; `maintenance.recurring`
(`service.go:45-52`) registers six kinds, none of them these. The certificate
sweep's own comment says it best: *"without this the table grows with fleet
size times renewals forever"* — and every agent renews at 80% of certificate
lifetime, so that table grows without bound at 10k agents by design.

### B5 — Two interval settings are fabricated projections
**HIGH / MEDIUM.** This is the same class as the recorded "a constant in an
adapter is a fabrication" lesson, now on the server side:

- **`inventory_interval_minutes` is fully resolved and never delivered.**
  Device override → MIN across groups → 1440 default
  (`device/proto.go:48,53-69`, `store/reads.go:406-408,1195-1211`), and the
  value **appears nowhere in `agent.proto`**. The agent's actual cadence is a
  hardcoded `time.NewTicker(24 * time.Hour)`. The resolved number feeds only
  `inventoryOverdue`, so an operator who sets the permitted minimum (120 min,
  `mutations.go:337`) marks that device **permanently overdue** while nothing
  changes on the device. Default and reality agree only by the coincidence
  that 1440 minutes is 24 hours.
- **`device_groups.sync_interval_minutes` is write-only.** It has an RPC, a
  permission and a group view (`devicegroup/state.go:271-286`,
  `control.proto:1199`) but **no query resolves it onto a device** — there is
  no counterpart to `ListGroupInventoryIntervalsForDevices`
  (`queries/devices.sql:230-236`). The proto comment ("0 = use device/default")
  implies a non-zero value applies. It never does.

### B6 — Three audit values that are asserted rather than observed
**MEDIUM.** Each writes evidence that was not measured:

- **Typed-nil notifier.** `webhook.New("")` returns `((*Client)(nil), nil)`
  assigned into an interface field (`cmd/cadestro/main.go:124-131`,
  `webhook/client.go:45-49`), so `s.notifier == nil` is never true. With no
  webhook configured, `InspectBackup` still records a `webhook / NOTIFY_INTENT`
  audit effect for a notification that is **structurally impossible**
  (`maintenance/service.go:403,453,464`); `Send`'s own `if c == nil`
  (`client.go:66`) is what hides it.
- **Hardcoded remainder.** `CleanupExpiredAuthStates`
  (`store/maintenance.go:25,35`) writes `AfterCount: &remaining` where
  `remaining` is a literal `int64(0)`, never queried. The chain records
  "0 remaining" as evidence of a count nobody took.
- **Understated erasure.** `EraseUser` (`store/user_erasure.go:18-95`) counts
  links, memberships, grants and the DEK, but `user_ssh_keys`,
  `tokens.owner_id`, `device_assigned_users` and `terminal_sessions` go by
  `ON DELETE CASCADE` with no effect row and no count
  (`schema.sql:43-44,231,289-291,360-363`). The rows do go — FKs are enforced
  — but the erasure record understates what was destroyed, which is precisely
  the record a subject-access request would be answered from.

### B7 — Unbounded growth on a schedule
**MEDIUM.** `VerifyAuditChain` (`store/audit_chain.go:267-315`) loads the
**entire retained chain into one slice** (two list queries bounded by
`toEnd = 1<<62`) and runs roughly five times an hour (hourly `audit.verify`,
plus `AnchorAudit` at 15-minute intervals and `RetainAudit`, both of which
verify first — `service.go:281,329`). `WriteAuditPrefix` was given paging
(`audit_archive.go:116-148`); the verifier was not. Separately the archive has
**no Delete and no cleanup**, and `verifyArchivedPrefixes` re-hashes *every*
checkpoint's prefix daily (`archive/archive.go:54-63`,
`maintenance/service.go:264-276`) — a daily cost proportional to total archived
bytes, rising forever. And retention deletes audit rows without deleting their
`search_documents` rows (`audit_chain.go:552-563`); not a disclosure (the
visibility SQL joins against live operations, `search.go:428`) but documents
plus two FTS5 shadow tables accumulate for rows that can never be returned.

### B8 — Two overclaims and a PostgreSQL ghost
**LOW, but they are claims about security properties.**
- *"Ordinary DELETE remains structurally impossible."* UPDATE is blocked on
  all four evidence tables and DELETE is guarded on operations/effects by an
  armed `audit_retention_guard` row (`schema.sql:916,934-973`) — but **the
  guard table itself has no trigger**, and **`audit_chain_head` is the only
  audit table with no trigger at all**, so nothing enforces its "only ever
  called with a height strictly greater" comment. Head rewriting alone still
  fails verification, so this is defense-in-depth, not a hole — but the
  sentence overclaims against a hostile in-process writer.
- *"the operator's **off-host** backup mount"* (`archive/archive.go:1-3`)
  while the enforced boot condition is only a different filesystem ID, and the
  archive path *is* `cfg.BackupPath`. Note `validateArchiveIsolation`'s own
  comment is scrupulously honest ("a floor, not a guarantee of remoteness") —
  one package comment claims what the deployment does not provide.
- `queries/audit.sql` carries two PostgreSQL-era comments: settings
  `pm.audit_retention_active` / `pm.audit_retention_up_to_seq` that do not
  exist, and "take the stream's head under a row lock" for a plain `SELECT`
  (SQLite has no `FOR UPDATE`; the real serializer is `Store.writeMu`,
  correctly documented elsewhere at `audit.go:221-223`).

### B9 — Corroboration: two independent readers found the same audit gap
`ListAuditEvents` recording nothing while `ExportAuditEvents` and
audit-scope `Search` both record a sensitive read (finding **A2** above) was
found independently by the RPC-surface scan and the internals scan
(`identity/audit.go:66-120` vs `:176-186` and
`searchrpc/handlers.go:231-239`). Same rows, same query, same permission, two
of three paths audited. Treat as confirmed.

### B10 — Smaller items
Validator asymmetry: four sibling interval RPCs, only the *device* inventory
setter re-checks its range in Go (`device/mutations.go:337-339`); the group
setter and both sync setters rely on the gotag alone
(`devicegroup/handlers.go:473-486`). Archive overwrite ordering: on the one
ref that is ever overwritten, the seal is replaced before the data
(`archive/fs.go:156-162`), so a crash in between leaves the old artifact with
the new seal — blast radius today is a wrong `SHA256` in `List`. Anchors below
a pruned boundary are silently skipped and never re-verified from the archive,
and exactly one off-host anchor object exists at a time
(`audit_chain.go:210-214`) — coherent, but it narrows the anchor guarantee to
"the newest position". And six recurring jobs (three at 15-minute intervals)
each audit their own transitions, appending ~50 chain rows an hour before any
user does anything — the reason retention exists, and the reason the chain can
never be quiet.

### Three wave-one open claims, now resolved
1. **Terminal `cols` defaulting — REFUTED.** `device/terminal_handlers.go:67-73`
   defaults cols/rows to 80/24 before minting, so the boundary's `omitempty`
   can never produce the zero the agent's `required,gt=0` would reject. Floor
   holds.
2. **Export redaction parity — CONFIRMED.** Both paths use the same view and
   the same `auditRowsToProto`, which structurally omits `sealed_detail`,
   `prev_hash` and `row_hash`. The defect sits next door: B9. Two latent
   asymmetries noted — `ListAuditEvents` exposes no date range though the
   filter fields exist, and `CountAuditEventRows` silently drops date bounds
   the list applies.
3. **sync vs inventory interval — the "direction" question was the wrong
   question.** Both are server-set; each has a *different* defect (B5).

### Where the floor holds (section B calibration)
The cryptographic core is sound and its comments match its code:
length-prefixed canonical encoding with presence markers
(`store/audit.go:562-581`), per-row-kind domain tags (`:644-647`),
verification recomputing from stored columns through the same encoder
(`:706-750`), and the walk compared against the recorded head so **tail
truncation is caught** (`audit_chain.go:186-197`).
`RecordPublishedAuditAnchor` refuses a tuple the chain cannot reproduce
(`:350-374`). `PruneAuditPrefix` requires digest, ref and timestamp *before*
opening a transaction and enforces the closed-prefix rule twice — trigger and
Go (`schema.sql:921-932`, `audit_chain.go:511-521`). `archive.Verify`
deliberately ignores the colocated sidecar and demands a digest from a
different trust domain (`archive/fs.go:215-231`). `VerifyAudit` fails closed
on a missing or contradicting anchor and on an absent archived prefix.
**Path traversal is closed on both artifact surfaces** (`archive/fs.go:64-76`
applied on Put *and* Get; `backupstatus/status.go:77` with `Lstat` +
`IsRegular` so a symlink is not followed), with size caps on the status marker
and doubly on the anchor object. Job leasing is correct — fresh claim ULID per
attempt, ownership checked on every transition, exact replays absorbed,
`ErrClaimLost` distinguished, handler panics recovered
(`jobs/state.go:133-357`, `runner.go:245-252`). The webhook is deploy-time
config only, https-only, no redirects followed, and its payload is a
three-field envelope from a closed event registry — **no user, device or
secret value is attachable** (`webhook/client.go:27-36,51-52,58,91,97-104`).
Search-document parity holds on every delete and erase path (29 refresh sites,
and an unknown resource type **errors** rather than skipping,
`search_documents.go:24-26`). And `store.withTx` holds a process-wide write
mutex across the callback *and* the audit append (`store.go:250-252`) — the
actual reason `chain_seq` is gapless.

---

## C. Agent executors and SDK backends

Dispatch coverage baseline: `ExecuteWithStreaming`
(`agent/internal/executor/executor.go:317-418`) wires every `ACTION_TYPE_*` in
the contract (SYNC is intercepted in the scheduler and never reaches the
executor). There is no reachable-but-unhandled action type.

### C1 — FILE idempotency drifted from its DIRECTORY twin, three ways
**CORRECTNESS BUG.** `fileMatchesDesired`
(`agent/internal/executor/action_file.go:212-221`) parses the desired mode
with `fmt.Sscanf(params.Mode, "%o", &desiredMode)` and compares against
`mode.Perm()`. The sibling `directoryMatchesDesired`
(`action_directory.go:138-146`) was **explicitly fixed** for archived agent#174
to use `strconv.ParseUint(...,8,32)` and to mask *both* sides with `.Perm()`.
FILE never got that fix, so it carries all three defects the directory's own
comment documents:

- **Unparseable mode is silently skipped.** `Sscanf` errors → the entire mode
  comparison is skipped → if content, owner and group match, the action reports
  *"already in desired state"* **without ever applying the mode**. The apply
  path (`fs.go:20-30`, `ParseUint`) would have rejected the same input. So one
  malformed value is a silent convergence lie when content matches, and a loud
  error when it doesn't.
- **Partial parse.** `"0777junk"` yields `0o777` with `err == nil` under
  `Sscanf`; `atomicWriteFile`'s `ParseUint` rejects it. Same divergence.
- **No `.Perm()` mask on the desired side.** `"4755"`, `"2755"`, `"1777"` give
  `desiredMode = 0o4755` against `mode.Perm() = 0o755` — never equal, so the
  action rewrites and re-chowns **on every single reconcile, forever**
  (`changed=true` permanently). The setuid bit is additionally dropped in the
  `os.FileMode` conversion, so it can never converge even in principle.

### C2 — An out-of-range rotation interval selects the most destructive behavior
**SECURITY-ADJACENT FAIL-OPEN.** Both rotation deciders treat the
"impossible" zero as *rotate always*:
- LUKS (`luks.go:414`): the interval check sits inside
  `if params.RotationIntervalDays > 0 { … }`; at zero the block is skipped and
  control falls through to an **unconditional** AddKey → StoreKey → RemoveKey
  on every cycle — continuous LUKS keyslot churn.
- LPS (`lps.go:315-317`): `intervalDuration = 0`, so
  `now.Sub(LastRotatedAt) >= 0` is always true → `(true, "scheduled")` every
  run.

The proto carries `required,gte=1,lte=365` and the server's compiler and
authoring validators descend into the params structs, so zero is rejected on
the normal path. But **the agent never re-validates incoming action struct
tags** (only stream queries at `handler.go:272,377` and repository params via
the SDK are validated). This is the round-one "validate-at-the-edge eroded
into validate-at-the-executor" pattern with the polarity wrong: an
out-of-range value should fail closed, not pick the most destructive branch.
Today it is defended only by trusting the control plane.

### C3 — Seven SDK capabilities are unreachable: ~3,750 implementation + ~6,260 test lines
**DEAD-UNREACHABLE, quantified.** `sdk/sys/{antivirus,catrust,dns,firewall,
netconfig,smart,timesync}` have no `ACTION_TYPE_*` in the contract, no dispatch
arm, and no non-test importer anywhere in `agent/` or `server/` — verified by
import sweep plus enum listing. Footprint: firewall ~1,472 + ~2,491 (three
backends, golden tests, security-machine tests), dns ~573 + ~998, netconfig
~648 + ~870, catrust ~415 + ~860, timesync ~232 + ~321, antivirus ~222 + ~454,
smart ~188 + ~270. All of it is built and tested in CI for zero reachable
function. Either wire the action types or delete the packages. (For
calibration: `notify`, `inventory`, `osquery`, `log`, `terminal`, `network`,
`remote`, `reboot`, `service`, `repo`, `encryption`, `user`, `desktop` are all
reachable — this is not a general indictment of `sdk/sys`.)

### C4 — `ActionResult.metadata` is a primed poison pill
**DEAD + LATENT TRAP.** Round one established the field is never populated and
the server hard-rejects it. This scan closed the loop on what would happen if
anyone did populate it: the agent marks an outbox row synced **only after**
`SendActionResult` returns success (`runtime.go:293-299`), and the server
refuses any result with non-empty metadata (`execution/result.go:77`). So a
metadata-bearing result would be rejected on every reconnect and **retried
forever** — a permanently stuck outbox row. Harmless while dead; the comment
inviting its use is the hazard.

### C5 — Config fragments are written live, then validated
**SECURITY GAP, minor.** `writeAndValidateConfig`
(`agent/internal/executor/helpers.go:107-129`, used by `sudo.go:97`,
`action_ssh.go:191,322`) writes atomically to the **final** path under
`/etc/sudoers.d/` or `/etc/ssh/sshd_config.d/`, *then* runs `visudo -c -f` /
`sshd -t`, removing the file on failure. There is a window in which an
unvalidated fragment is live and consulted. Modern sudo and sshd skip a
parse-erroring drop-in with a warning, which bounds the blast radius, but the
safe shape is validate-a-temp-then-rename. Consistent across both call sites.

### C6 — FILE can write the very files whose syntax gates it bypasses
**FOOTGUN (within the trusted-operator model).** The FILE PRESENT branch
blocks only `isCriticalFile` (13 named files), deliberately *not*
`IsUnderProtectedPrefix`, so managed `/etc/*.d` config remains writable
(`action_file.go:41-55`). Consequence: a FILE action can write
`/etc/sudoers.d/*`, `/etc/ssh/sshd_config.d/*` or `/etc/cron.d/*` with **no**
`visudo`/`sshd -t` validation — the exact pre-commit check ADMIN_POLICY and
SSHD apply to the same paths. A malformed fragment delivered this way can lock
out sudo or ssh with no backstop.

### C7 — Smaller correctness notes
Managed-block FILE has **no BEGIN/END markers** (`action_file.go:71-88`): it
appends `params.Content` and checks `strings.Contains` for idempotency, so
editing a block's content appends a second copy and leaves the first — the
action is idempotent only for byte-identical content. GROUP and USER do not
reconcile identity drift: `setupGroup` (`group.go:60-70`) sets GID and the
system flag only at creation, and `updateUser` never changes UID, so a
divergent GID reports "already up to date" — defensible safety choices, but
silent ones (related to the round-one `primary_group`/`gid` items). And
`skip_if_unchanged`'s hash keeps `Output` (`store/manifest.go:434-446`),
confirming from the store side that the suppression essentially never fires
for any action with varying stdout.

### Where the floor holds (section C calibration)
This is the strongest calibration result of either wave, and it covers the
paths that touch disk encryption and self-update:

- **LUKS ordering is correct**: AddKey → StoreKey (with rollback RemoveKey on
  store failure) → `verifyKeyRoundTrip` (re-fetch, exact match, volume test) →
  RemoveKey of the old key/PSK, behind a pre-mutation `requireLuksStoreReady`
  gate, with fail-closed state reads, a correct conflict comparator, and both
  the timestamp hot-loop escalation and initial-timestamp fail-loud guards
  from archived agent#80/#173.
- **LPS reports before it sets**: sealing and `StorePasswords` complete before
  `SetPassword`, so a failed report leaves the account unchanged; passwords are
  `exec.Secret`, never in output or metadata.
- **Sealing is textbook ECIES** (`sdk/crypto/seal.go`, `field_context.go`):
  per-call ephemeral X25519, HKDF-SHA256 salted with both public keys,
  **mandatory** non-empty AAD *and* info (fail-closed), AES-GCM,
  length-prefixed unambiguous encoding, malformed and empty inputs rejected;
  credentials opened with `defer clear()`.
- **Self-update is a proper signed chain**: Ed25519 verification of the
  SHA256SUMS manifest **before** any hash in it is trusted → pinned binary
  hash → integrity-verified download → version self-test in a subprocess →
  atomic swap with a copy-not-move backup so the live binary survives any
  failure; anti-rollback with fail-closed version parsing; BYOK key
  fail-closed on the unstamped placeholder.
- **DEB/RPM/AppImage** all require `requireVerifiedArtifact` (https + 64-hex
  checksum) before any download or remount, with pin-aware redirect policy and
  safe filename derivation.
- **SSH/SSHD/USER key handling**: CR/LF/NUL rejection is fatal rather than
  skipped; `validateActionIDForFilesystem` guards every path-splicing action;
  the authorized_keys write is `O_NOFOLLOW` + `SafeReplaceFile` + FD-based
  chown/chmod — symlink-race hardened.
- **Scheduler recovery**: manifest dedup rejects same-ID/different-bytes and
  accepts identical replay; `RecoverInterruptedOccurrences` resolves STARTED
  rows to INDETERMINATE **without repeating side effects** and proves reboot
  completion by kernel boot-ID change; the outbox is at-least-once against an
  idempotent server.
- **The SDK validators hold on spot-check**: leading-alphanumeric
  anti-option-injection across package, rpm and remote names;
  `ValidateGpgKeyRef` refuses rpm's `ext::` RCE transport and plaintext http;
  `IsAllowedEnvVar` blocks the `LD_*`, `BASH_FUNC_*`, `DYLD_*` families plus
  `GCONV_PATH`, `PATH`, `IFS`, `BASH_ENV`, `PYTHONPATH`, case-insensitively.

---

## D. Web forms, session posture, deploy, and CI

### D1 — The one field whose entire purpose is integrity is authored as optional
**CORRECTNESS + SECURITY-ADJACENT.** `AppInstallParams.checksum_sha256` is
`required,len=64,hexadecimal`, and the proto says why: *"without it the agent
installs a binary whose only authenticity is TLS to a possibly-compromised
origin"* (`contract/proto/cadestro/v1/actions.proto:166-170`). The form field
(`web/src/lib/components/actions/forms/AppParamsForm.svelte:25`) has **no
`required`, no format check, no `aria-invalid`, and no `FieldError`** — while
the URL field directly above it has all of them. The server rejects it, but
the field *looks* optional and its rejection cannot render on the field. An
operator authoring an app install is being invited to skip the checksum.

### D2 — Client validation is uniformly a subset of the contract
**PATTERN (verified across all 18 forms).** Number inputs mirror proto bounds
exactly; string, enum and bool fields lean almost entirely on server
rejection. The drift is therefore always *permissive* — the form admits what
the server refuses — and the recurring sub-defect is missing inline
`FieldError` plumbing, so a field-keyed server rejection degrades to a generic
toast. Instances beyond D1:
- **Cron is display-only** (`ActionScheduleForm.svelte`): no grammar check at
  all, and `describeCron` (`:34-92`) silently returns `''` for 6/7-field or
  descriptor forms rather than flagging them. Round-one item 64 at the UI
  surface — invalid cron reaches the agent and fails open to the 8-hour drift.
- **Self-lockout is authorable** (`SshParamsForm.svelte:54,62`): `allowPubkey`
  and `allowPassword` are independent switches, and the preview (`:25-29`)
  *renders* the resulting `PubkeyAuthentication no` + `PasswordAuthentication
  no` without blocking it.
- **`type="url"` accepts `http://`** on both agent-update URLs
  (`AgentUpdateParamsForm.svelte:34,44,59,68`) where the contract demands
  `startswith=https://`. Also: `allow_downgrade` has **no control at all** in
  the form (`types.ts:250-256`) — that proto field is unreachable from the web.
- Empty custom sshd directive values pass the form
  (`SshdParamsForm.svelte:106-115`) and fail at `required,min=1`.

### D3 — The `?? true` dead-fallback class is larger than round one measured
**DEAD-UNREACHABLE.** Eleven live in `forms/types.ts`
(`:799,826,845,859,894,895,912,913,914,926,986`) plus
`edit-params-dialog.svelte:88` — not seven. All sit on the proto→form decode
path, where protobuf-es scalar bools decode `false` and never null, so the
branch cannot fire. Harmless in effect, but it is eleven expressions encoding
a false belief about the wire format.

### D4 — Assign dispatch gap, confirmed at the UI surface
**GAP (corroborates the round-one finding).** `assignSetToGroup`
(`web/src/routes/(app)/assign/assign-data.ts:140-148`) writes the
`DEVICE_GROUP` assignment and dispatches nothing; `commitAssign` (`:160-182`)
calls `dispatchActionSet` only per-DEVICE and only for `schedule === 'now'`.
The file's own header (`:35-37`) documents the refusal — *"Nothing is
dispatched for a rule target … this surface refuses to guess a fan-out."*
Under your ruling that `DispatchAssignedActions` should not exist, this
comment is describing the right instinct at the wrong layer: the UI is
correct not to guess a fan-out; the server should be compiling on sync.

### D5 — Both tokens live in web storage, not just the access token
**Sharpens the HttpOnly case.** `contract/ts/auth.ts:11,42,61-69` stores a
superjson blob under `cadestro-auth` — access **and refresh** token — in
`localStorage` when "keep me signed in" is set, else `sessionStorage`. Round
one framed the exposure as the access token; the durable **refresh** token is
the more valuable target, and any XSS reads both. Rotation is present
(`doRefresh` replaces both, `:222-238`) and `scheduleRefresh` reads
`expiresAt` (`:174-190`) — the exact client-side friction the cookie move has
to solve.

### D6 — Release workflow grants write permissions to every job
**SUPPLY-CHAIN LEAST-PRIVILEGE GAP.** `.github/workflows/release.yml:30-34`
sets `contents: write, packages: write, id-token: write, attestations: write`
at the **top level**, so `server-test`, `agent-integration`, `server-build`
and `agent-build` — which only test and upload artifacts — run with
release-grade write plus OIDC minting. A compromised dependency executing
during `go test` would hold exactly that. The correct pattern already exists
in the same file: `web-image` (`:238-240`) narrows to `contents: read,
packages: write`. Fix shape: top-level `contents: read`, elevate per
publishing job.

### D7 — The control container runs as root; the web container does not
**SECURITY GAP (least privilege).** `Containerfile.control` sets no `USER`,
while the web image correctly drops to `USER bun`. Ports 8081/8082 are
unprivileged, so root is not needed to bind. The current design leans on it —
`smoke-test.sh:31-33` records that control and `backup.sh` write root-owned
volume content — so hardening means aligning volume uid/gid, not just adding
a `USER` line.

### D8 — The deploy tree hardcodes `PRAGMA user_version == 1`
**LATENT, and it collides with your SQL-upgrade direction.** `backup.sh:50`
and `smoke-test.sh:184-185` both test the schema version literally. Correct
today; the day the schema bumps, **the backup path breaks before the upgrade
path does** — which is the sharpest practical constraint on the post-RC
manual-upgrade direction you recorded in round one.

### D9 — Launch blocker #1, sharpened by its own docref anchors
`README.md:39` and `docs/installation.md:36` tell operators to fetch
`releases/latest/download/install.sh` to install **control**;
`release.yml:314` publishes the **agent** installer under that exact name, and
the deploy tree is never a release asset. The sharpening:
`docs/installation.md` **docref-anchors `server/deploy/install.sh` functions**
(`:19,40,59,109`) while the command on the same page fetches the agent asset —
the anchoring made the page feel verified precisely where it was wrong. That
is round-one pattern #5 (docs written from code inherit the code's blind
spots) in its purest form. Related: SBOM and provenance cover all four
binaries and `SHA256SUMS`, but **not** the deploy tree — so even a
verification step in the server installer would have nothing signed to check
against until the tarball becomes an asset.

### D10 — Smaller deploy items
`install.sh:129-133` still downloads a source archive over TLS-only trust with
a **mutable branch fallback** and no signature check. `IMAGE_TAG` defaults to
`latest` two lines under `.env.example`'s own "Pin a release in production",
and `install.sh:157-160` derives a pin only for `v*` tags — so a branch
install runs branch source against moving `latest` images. Images are pulled
by mutable tag, never digest. And `install.sh:8` defaults
`GITHUB_REPOSITORY=MANCHTOOLS/cadestro` (uppercase) against lowercase
everywhere else — works only via GitHub's case-insensitive redirect.

### Reconciling one apparent conflict between two wave-2 readers
This scan reports that `backup.sh` records **sha256 + `integrity_check` +
`foreign_key_check` + size>0**, and calls round one's "size-not-hash" finding
addressed. Both are true and they are about different halves: the **writer**
(`backup.sh:69-72`) records a hash; the **reader**
(`server/internal/backupstatus/status.go:81-84`) compares only
`artifactInfo.Size() != stored.SizeBytes` and validates the recorded digest
for *shape* only, never recomputing it. So the original finding stands — the
continuous check does not verify integrity — and it is smaller than it looked,
because the digest needed to do so is already recorded at backup time.

### Where the floor holds (section D calibration)
**Fork-publish is safe**: release runs only on `push: tags: ['v*']` in the
pushing repo's context, the signing key sits behind `environment: releases`,
and there is **no `pull_request_target` anywhere**; `deploy-smoke.yml` on
`pull_request` carries only read scopes. **Path filters are correct** —
server/sdk/agent fan in on `sdk/**` + `contract/**` because they `replace` to
those directories, `web.yml` watches the two contract paths its aliases
resolve to, and `docref.yml` is deliberately unfiltered. The **web security
posture is genuinely strong**: nonce-based CSP with `frame-ancestors 'none'`
(`svelte.config.js:19-34`) plus HSTS, `X-Frame-Options: DENY`,
Referrer-Policy and Permissions-Policy (`hooks.server.ts:50-54`); the
control-URL injection is scheme-validated and attribute-escaped; the terminal
carries its ticket in the **WebSocket subprotocol**, never the URL, and
rejects `ws://` client-side (`Terminal.svelte:137-149`); the dev-auth bypass
is gated on compile-time `import.meta.env.DEV` and eliminated from production
builds. The **form default factories deliberately compensate for the
proto3-bool lie** — `systemWide`, `recursive`, `createHome`, `allowPubkey`,
`dnf.enabled`, `gpgcheck`, `autoConnect` all default `true` in the form
(`types.ts:277-427`), so a web-authored action is safe-by-default even though
the wire default is not. And the **deploy shell layer is exemplary**:
`umask 077`, a self-discovering permission gate over discovered secret files,
env parsed rather than sourced, `openssl -checkhost` matched on stdout rather
than its non-portable exit code, archive isolation enforced before any key
material, Traefik access logs stripped of request paths and headers, and a
`smoke-test.sh` that refuses a docker.sock mount, derives archive isolation
from resolved mounts, asserts both expected **and forbidden** RPC surfaces,
refuses a direct agent connection lacking PROXY v2, and canaries a
query-string credential through the Traefik logs.

---

## E. Mutation kernel, authorization containment, search, tokens, SCIM

This scan's headline is a **privilege-containment gap nobody else found**, set
against the strongest calibration result in either wave: the product's central
structural claim holds.

### E1 — A holder of `AssignRoleToUser` can grant themselves Admin
**SECURITY GAP / needs your ruling. MEDIUM.** `identity/roles.go:290-356`
(`AssignRoleToUser`) and its scope gate `checkGrantScope` (`:582-642`) never
check that the granting actor's own permission set covers the role being
granted. The escalation guards that exist protect only the *scoped* path:
privilege-granting roles cannot be scoped (`roles.go:627-630`), and a
scope-limited admin cannot mint an unscoped grant
(`auth.EnforceUnscopedGrantAuthority`, `scope.go:215-229`). But a principal
holding `AssignRoleToUser` with **no** `AssignRoleScope` grant is classified
"ordinary admin — allowed" (`scope.go:228`) and may grant **any** role
globally, including built-in Admin, **to themselves**.

Consequence: `AssignRoleToUser` cannot be delegated as a limited capability —
holding it is equivalent to holding full admin. `AssignRoleToUserGroup`
(`roles.go:428`) shares the gap. For a codebase that invests this heavily in
scope-limited delegated administration, this wants an explicit decision:
either document both RPCs as admin-equivalent, or require that the granter
hold every permission in the role they are granting.

### E2 — Certificate-revocation cleanup is dead code (second independent finding)
**CORRECTNESS + COMMENT DRIFT.** `store/revocation.go:64-78` defines
`DeleteExpiredRevocationsInTx`, whose comment states *"without this the table
grows by one row per certificate rotation forever … it runs inside a
BACKGROUND_WRITER-class audited operation"*. It has **no caller anywhere**,
and the maintenance registry (`maintenance/service.go:27-32,98-116`) has no
revocation-cleanup kind. Meanwhile rows are written on every certificate
renewal and every device delete (`device/mutations.go:389` via
`store.RevokeInTx`). Not a security hole — expired certificates fail the
`notAfter` check regardless and `IsRevoked` is an indexed lookup — but it is
monotonic disk growth for the life of the deployment, and the comment asserts
wiring that does not exist. This is the same finding as **B4** reached
independently; treat as confirmed.

### E3 — Two more corroborations
`RecordPublishedAuditAnchor` and `PruneAuditPrefix` bypassing `WithAudit`
(**B1**) and `UpdateServerSettings` being a full-replace over two proto3 bools
(the meta-sweep's inverted-defaults class) were both found again here
independently. On B1 this reader adds a fair counterpoint worth recording: a
normal audited mutation *over the audit tables* would be self-referential, and
the checkpoint and anchor rows are themselves append-only evidence — so the
silence may be deliberate. It is still an asymmetry with the stated
"every background writer has structurally enforced audit coverage" property,
and it is the one job that destroys evidence, so it belongs in the design doc
explicitly rather than by omission.

### E4 — Executions of a soft-deleted device leave stale index rows
**HYGIENE, not exploitable.** `DeleteDevice` records only a `device`/DELETE
effect, which refreshes that device's document but not its executions'
(`store/search_documents.go:34-42`). Those linger until a full
`RebuildSearchIndex`. Reads are safe: the executions visibility predicate
requires `EXISTS(… devices d WHERE d.id = base.device_id AND d.is_deleted =
FALSE)` (`store/search.go:422-427`), so they are dropped at read time. Index
bloat only.

### The central claim, verified holding
The product's structural thesis — *a state change without its audit row is
impossible* — was traced end to end and **holds**:

- The write door is closed structurally. `withTx` is unexported
  (`store.go:250`), and its only in-package callers are `WithAudit` /
  `WithAuditEffects` (`audit.go:243,353`), the two audit-infrastructure
  writers, and **one named, documented exception** (heartbeat telemetry,
  `heartbeat_telemetry.go:19-35`, labelled "sole unaudited state writer").
- No `*sql.DB`, `*sql.Tx` or `generated.New` handle escapes the store package.
  Domain packages import `store/generated` for parameter *types* only and can
  mutate solely through the `*store.Tx` passed into a `WithAudit` callback —
  whose `raw *sql.Tx` is unexported.
- `WithAudit` appends the operation row before a single commit; any failure,
  including in the audit write itself, rolls back the whole mutation.
- Append-only is database-enforced (UPDATE/DELETE triggers on operations,
  effects, anchors and checkpoints, `schema.sql:934-972`), and the retention
  "unlock" is a transaction-local guard row armed and disarmed inside
  `PruneAuditPrefix` — with a closed-prefix trigger and a Go-side
  stranded-effect recount on top.

Also verified sound in this pass: out-of-scope device and user access returns
**NotFound**, never PermissionDenied — no existence oracle
(`device/handlers.go:158-195`, `identity/users.go:52-77`); every mutating
device RPC routes through `mutationDevice`; `checkPermissionKeys`
(`roles.go:549-565`) rejects unregistered keys, which genuinely closes the
historical `UpdateUserLinuxUsername:self` bypass documented at
`permissions.go:144-150`; privilege-granting classification fails closed on
unknown keys; session invalidation on role and membership change is
in-transaction and thorough; SCIM group patch/sync reconcile all members in
one audited transaction with provider-ownership enforcement; JWT pins EdDSA
and rejects alg-substitution and `none`; and the rate limiter's over-limit
calls do not accrue.

### One reconciliation between wave-2 readers
This scan judges the `audit_events` search facet safe, noting it is gated by
the `ListAuditEvents` facet check and excluded from default-scope search. The
RPC-surface scan (**A3**) judges the same code fragile, because that exclusion
is the positional slice `searchFacets[:8]` with no test. Both readings are
correct and they are about different questions: the gate **works today**, and
it is **held in place by nothing but array order**. The finding stands as
written in A3.

