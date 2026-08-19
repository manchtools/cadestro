# The policy model

This is the page to read before any other. Everything Cadestro does to a device
is an instance of one mechanism, and the mechanism has three moving parts:

1. **Authoring** — you declare *desired state* as actions, group them, and give
   them a schedule.
2. **Assignment** — you attach what you authored to devices or groups. When a
   dispatch is requested for a device, the server resolves those attachments,
   compiles them into a flat, ordered **manifest**, and commits one **delivery**
   row.
3. **Execution** — the agent durably records the delivery, then enforces it on
   its own schedule, with or without a connection to the server.

The rest of this page is those three parts in detail, and then the two things
that behave differently on purpose: one-shot dispatch, and compliance.

---

## 1. Desired state, not steps

An action declares what should be true, not what to run. The state is a
two-valued enum, and it is the same enum for every stateful action type —
packages, files, directories, users, groups, services, repositories, SSH access,
privilege policies:

<!-- docref: begin src=contract/proto/cadestro/v1/common.proto#DesiredState:570e334d -->
`DESIRED_STATE_PRESENT` (the wire default, value `0`) means install, create, or
enable. `DESIRED_STATE_ABSENT` means remove, delete, or disable. There is no
third value and no "latest"/"any" escape: an action either asserts a thing is
there or asserts it is gone, and the agent's job on every run is to close the
gap between that assertion and the machine.
<!-- docref: end -->

The consequence worth internalising: **an action is re-run for its whole
lifetime, not once.** Removing an action from an assignment stops enforcing it;
it does not undo it. To undo, you set the state to `ABSENT` — or use the
assignment mode that does it for you:

<!-- docref: begin src=contract/proto/cadestro/v1/common.proto#AssignmentMode:d94753c7 -->
An assignment carries a mode. `REQUIRED` (the default) applies the action.
`AVAILABLE` offers it for selection rather than applying it. `EXCLUDED` blocks
it on that target. `UNINSTALL` applies the action but forces its desired state
to `ABSENT` — which is how you retire something from a group without editing the
action every other group still shares.
<!-- docref: end -->

Whether the agent actually changed anything is reported separately from whether
it succeeded. A run that finds the machine already correct returns success with
`changed = false`, which is what makes re-running cheap and what
`skip_if_unchanged` (below) keys off.

### Schedules

<!-- docref: begin src=contract/proto/cadestro/v1/actions.proto#ActionSchedule:2f9d3987 -->
A schedule is four fields. `cron` is a cron expression (`"0 3 * * *"`);
`interval_hours` is used instead when `cron` is empty, and is the plain
drift-prevention cadence; `run_on_assign` runs the work immediately the first
time it is received rather than waiting for the next firing; and
`skip_if_unchanged` suppresses the result report when a run succeeded and
changed nothing, so a fleet re-asserting the same state every few hours does not
bury the interesting results.
<!-- docref: end -->

One subtlety that trips people up. An `Action` has a `schedule` field, but
**that field is authoring data, not an execution trigger** — the agent fires on
the schedule of the *manifest* the action was compiled into. A single action
assigned on its own becomes a one-action manifest that simply carries that
schedule at the manifest level, so the distinction is invisible until you group
actions, at which point the group's schedule is what runs.

<!-- docref: begin src=agent/internal/store/store.go#calculateNextExecuteFromSchedule:f4df1dad,agent/internal/store/store.go#cronParser:d1ef0c24,agent/internal/store/store.go#nilScheduleDrift:5d0fcdc7 -->
Two concrete details worth knowing before you write a schedule. The cron parser
takes the **standard five fields** — minute, hour, day-of-month, month,
day-of-week. There is no seconds field and no `@daily`-style descriptor. And a
manifest with no schedule at all is not a manifest that never runs: it falls back
to an **eight-hour drift-prevention cadence**, which is the sane default for
"keep asserting this".

The failure mode is worth stating plainly because it is fail-*open*: a cron
expression the agent cannot parse is logged as a warning and then falls back to
that same interval behaviour. It does not stop the action, and nothing rejects a
malformed cron when you author it — so a typo becomes "runs every eight hours"
rather than an error you notice.
<!-- docref: end -->

---

## 2. From what you authored to what the agent gets

### The authoring hierarchy

<!-- docref: begin src=server/internal/manifest/compiler.go#Compiler:acd57947 -->
There are three authoring levels and they compose in one direction only: an
**Action** is a single declaration; an **ActionSet** is an ordered list of
actions; a **Definition** groups action sets. The compiler is what turns any of
them into executable work.
<!-- docref: end -->

Assignments name which of those they attach, and to what:

<!-- docref: begin src=contract/proto/cadestro/v1/common.proto#AssignmentSourceType:a865d1eb,contract/proto/cadestro/v1/common.proto#AssignmentTargetType:09b782e1 -->
The four assignable sources are `ACTION`, `ACTION_SET`, `DEFINITION`, and
`COMPLIANCE_POLICY`. The four targets are `DEVICE`, `DEVICE_GROUP`, `USER`, and
`USER_GROUP` — so work reaches a machine either by naming it, by naming a group
it belongs to, or by following a user (or user group) to the devices that user
reaches.
<!-- docref: end -->

### Creating an assignment does not by itself dispatch it

This is the single most important operational fact on this page, and it is easy
to assume the opposite.

<!-- docref: begin src=contract/proto/cadestro/v1/control.proto#ControlService.DispatchAssignedActions:031cd7e5,server/internal/dispatch/handlers.go#Handlers.DispatchAssignedActions:50b74470 -->
Writing an assignment row commits the *intent*. An authenticated agent's next
`Sync` resolves that device's assignments and pulls the current desired policy;
`DispatchAssignedActions` remains only as a compatibility sync hint and never
commits a policy delivery. There is no background reconciler that walks the
fleet applying new assignments on its own.
<!-- docref: end -->

<!-- docref: begin src=server/internal/dispatch/assigned.go#CompileAssigned:f3bb04b0 -->
When it does run, resolution walks the assigned sources in a fixed order —
definitions, then action sets, then singleton actions, then compliance policies
— and an action already carried by a container it walked earlier is not emitted
a second time. So a definition and a bare action that both reference the same
action produce one occurrence, not two.
<!-- docref: end -->

### When two assignments disagree

<!-- docref: begin src=server/internal/assignment/handlers.go#ResolveSources:d8fae4a1,server/internal/dispatch/assigned.go#forceAbsent:7efc3ff9 -->
For **one source reached by several paths**, the modes collapse by a fixed
precedence: `EXCLUDED` beats everything and the source is dropped; `UNINSTALL`
beats `REQUIRED` and rewrites every occurrence of that source's manifest to
`ABSENT`; `AVAILABLE` contributes only when the user has selected it.
<!-- docref: end -->

For **two different sources carrying the same action with different desired
state**, there is no resolution rule in the code. Because an action absorbed by
an earlier container is skipped, the outcome follows the emission order above
rather than a declared policy. Do not model that as a precedence rule — model it
as something to avoid authoring.

### Compilation flattens it

The agent never receives a tree. Compilation resolves the hierarchy to a flat
list before anything leaves the server:

<!-- docref: begin src=server/internal/manifest/compiler.go#Compiler.ActionSet:7b8db1be,server/internal/manifest/compiler.go#Compiler.Definition:c58ccefb -->
Assigning an Action produces one singleton manifest. Assigning an ActionSet
flattens the set into one manifest, in authored member order. Assigning a
Definition produces **one globally ordered runbook** — the definition's schedule
overrides the runbook without rewriting the underlying sets, and each set keeps
its own failure policy on its occurrences.
<!-- docref: end -->

A set or definition member that contains no live actions cannot become
executable work, and compilation refuses it rather than emitting an empty
manifest.

Secret-bearing parameters are sealed during compilation, per device:

<!-- docref: begin src=server/internal/manifest/compiler.go#Compiler.ActionForDevice:63266f1a -->
The `…ForDevice` compile path seals every classified field to the target
device's enrollment key *before* the manifest is persisted, so the durable
delivery row never holds a plaintext secret and a manifest compiled for one
device cannot be opened by another.
<!-- docref: end -->

See [Security model → secret handling](security-model.md#5-secret-handling) for
what the seal binds.

### The manifest

<!-- docref: begin src=contract/proto/cadestro/v1/agent.proto#Manifest:0525bf5a -->
A manifest is the unit of assignment the agent executes: a flat, ordered list of
action occurrences under **one** schedule and **one** default failure policy.
Execution order is list order.
<!-- docref: end -->

<!-- docref: begin src=contract/proto/cadestro/v1/agent.proto#ManifestOccurrence:77b5cbac -->
Each position in that list is an *occurrence* with its own `occurrence_id`.
Occurrence identity is not delivery identity: the occurrence id names a position
the author created, the delivery id names one attempt to hand the manifest to a
device. Duplicate authored occurrences — the same action reached twice through
two sets composing one definition — are deliberately **preserved and executed at
each position**, not collapsed, and the distinct occurrence ids are what keeps
their results apart.
<!-- docref: end -->

<!-- docref: begin src=contract/proto/cadestro/v1/agent.proto#ManifestProvenance:0f9408ba -->
Every manifest carries a bounded provenance record — at most one definition, one
action set, and one action id, in that order. It is a path, never a tree: which
levels are populated tells you which assignment produced this manifest, and the
agent neither receives nor resolves a recursive composition.
<!-- docref: end -->

<!-- docref: begin src=contract/proto/cadestro/v1/agent.proto#OnFailure:7c926349 -->
Failure policy is per occurrence, resolved from the set's declaration at
compile time so the agent needs no fallback. `ON_FAILURE_CONTINUE` is the wire
default and runs the remaining occurrences; `ON_FAILURE_STOP` abandons them.
<!-- docref: end -->

<!-- docref: begin src=agent/internal/scheduler/scheduler.go#Scheduler.executeManifest:c462e71c -->
When a manifest runs, the agent walks the occurrences in order. After a failure
under `STOP`, the remaining occurrences are not silently dropped — each still
gets a result row, with status `SKIPPED` and the reason recorded. Statuses
`SUCCESS`, `NOT_APPLICABLE`, and `SKIPPED` do not trip the stop; anything else
does. The manifest's aggregate status is `SUCCESS` when every executed
occurrence succeeded, `FAILED` when any did not, and `INDETERMINATE` when a
crash left an effect unknown.
<!-- docref: end -->

---

## 3. Delivery: durable, and acknowledged by the device

This is the part that distinguishes Cadestro from a push that hopes for the
best. **A successful socket write is never treated as delivery.**

### The state machine

<!-- docref: begin src=server/internal/delivery/state.go#StatePending:5db58134 -->
A delivery row moves through `PENDING` → `PUSHED` → `ACKED_RECEIPT` → one of
`SUCCEEDED` / `PARTIAL` / `FAILED`. A pushed delivery becomes available for retry
30 seconds later, so a stream that dies mid-write is re-sent rather than lost.
<!-- docref: end -->

<!-- docref: begin src=server/internal/delivery/state.go#StateExpired:5db58134 -->
Two further terminal states, `EXPIRED` and `CANCELLED`, are declared and
accepted by the state machine but **nothing in the current server writes them**.
A delivery to a device that never comes back stays pending indefinitely rather
than ageing out. Cancelling an execution cancels the execution row, not its
delivery.
<!-- docref: end -->

<!-- docref: begin src=server/internal/delivery/state.go#InsertInTx:c61c90d8 -->
The row — with the entire compiled manifest in it — is committed inside the
audited transaction of the operation that created it, before any frame is sent.
The `delivery_id` is minted at that commit and never changes: every transport
retry, reconnect, and sweep re-send carries the same id.
<!-- docref: end -->

### The receipt is the exactly-once boundary

<!-- docref: begin src=contract/proto/cadestro/v1/agent.proto#DeliveryReceipt:ca3627eb,agent/internal/scheduler/scheduler.go#Scheduler.RecordDelivery:921ce6b3 -->
The agent sends a `DeliveryReceipt` **only after it has committed the delivery
to its own local store** — never on arrival. Its insert reports whether the row
was new; a redelivery of an id it already holds is recognised as a repeat, so
the work is executed once no matter how many times the frame arrives.
<!-- docref: end -->

<!-- docref: begin src=contract/client.go#Client.runManifestDelivery:61cb4208 -->
The ordering is structural, not a convention someone has to remember: the
receipt is emitted by the same function that invoked the commit, only on its
success return. There is no code path that emits a receipt without the commit
having landed, so a handler error — or a panic — leaves the delivery
unacknowledged, which is exactly the state that makes control redeliver it.
<!-- docref: end -->

<!-- docref: begin src=agent/internal/store/manifest.go#Store.RecordManifestDelivery:636d839b -->
The agent's dedupe is byte-exact, not id-only. A replay carrying identical
manifest bytes is accepted without touching the schedule or execution state. The
same delivery id carrying *different* bytes is **rejected** — and because the
receipt only follows a successful commit, control never sees an acknowledgement
for it. A rewritten delivery cannot quietly replace work the device already
recorded.
<!-- docref: end -->

<!-- docref: begin src=server/internal/delivery/state.go#Service.AcknowledgeReceipt:74efb523 -->
On the server side, that receipt — and nothing else — advances the delivery out
of `PUSHED`. A write the device never persisted must be retried, and the receipt
frame is the only thing that distinguishes the two cases. Replays after the
transition are successful no-ops, and a receipt naming another device's delivery
is rejected outright.
<!-- docref: end -->

<!-- docref: begin src=server/internal/delivery/state.go#Service.MarkPushed:ec61132f,server/internal/delivery/state.go#Service.Complete:33b7134a -->
Every transition is a conditional, audited SQLite transaction rather than a
read-then-write. Pushes carry the connection's epoch so an older stream can
never overwrite a newer one's state, and a completion replay is accepted only if
it agrees with the state and result code already committed — a second, different
verdict for the same delivery is refused, not overwritten.
<!-- docref: end -->

### How a delivery finds its device

<!-- docref: begin src=server/internal/delivery/dispatcher.go#Dispatcher.Run:d920988c,server/internal/delivery/dispatcher.go#Dispatcher.Wake:5f1eb5cd -->
Dispatch is durable database state plus an in-process wakeup. Committing a
delivery queues its id on a bounded in-memory channel, which a small pool of
workers drains. The queue is best-effort by design: a full queue or a dropped
notification loses nothing, because a periodic sweep re-queues every due
delivery for every currently connected device on a 30-second tick. The database
is the correctness path; the wakeup is only the latency path.
<!-- docref: end -->

<!-- docref: begin src=server/internal/delivery/dispatcher.go#Dispatcher.Dispatch:2b7e41c8,server/internal/delivery/dispatcher.go#Sendable:5d02d815 -->
A device that is offline is not an error: the row stays durable and a reconnect
notification or the next sweep retries it. A reconnecting agent does not wait
out the previous stream's retry delay — a pushed-but-unacknowledged delivery is
re-sent immediately on the newer connection epoch — while a delivery scheduled
for the future is never pulled forward. The same guard is used by the push path
and the agent's sync path so the two cannot drift apart.
<!-- docref: end -->

<!-- docref: begin src=contract/proto/cadestro/v1/agent.proto#SyncState:42f97884 -->
Periodic sync is a *state refresh*, not a second dispatch path. It carries every
explicit delivery currently assigned to the device and the device's resolved
desired policy snapshot. Explicit deliveries retain their durable
`delivery_id`; assignment policy is reconciled locally as replaceable scheduled
work without a transport receipt. The default sync interval is 30 minutes
unless the server sets one.
<!-- docref: end -->

<!-- docref: begin src=contract/proto/cadestro/v1/agent.proto#AgentService:1027f6e5 -->
All of this rides a single bidirectional stream. There is one agent–control
transport: handshake, sync, dispatch, results, secret operations, and terminal
traffic all use it. There is no second channel and no inbound port on the
device.
<!-- docref: end -->

---

## 4. Maintenance windows

<!-- docref: begin src=contract/proto/cadestro/v1/common.proto#MaintenanceWindow:528399a5 -->
A maintenance window gates dispatch by **device-local wall-clock time**. It is a
positive allowlist and it is opt-in: an empty schedule means "always allowed", so
a group with no window configured behaves exactly as it did before windows
existed. Multiple entries combine as OR.
<!-- docref: end -->

<!-- docref: begin src=contract/proto/cadestro/v1/common.proto#MaintenanceWindowEntry:7c3003a6 -->
An entry is a set of weekdays (`mon`…`sun`) plus one 24-hour clock range in
`HH:MM-HH:MM` form. Ranges that cross midnight are supported — when the start is
greater than the end, the window continues into the next day.
<!-- docref: end -->

Timezone handling is the deliberate design decision here. The server never tries
to interpret the device's timezone: it computes the union of every window
reaching the device (its own device groups plus any user groups that reach it
through an assignment) and ships that, and the agent evaluates it against its own
local clock. "02:00 local" means 02:00 wherever the machine actually is.

<!-- docref: begin src=agent/internal/scheduler/scheduler.go#Scheduler.dispatchAllowed:fcb93355 -->
The window is evaluated at dispatch time and **fails closed**: the agent
persists the resolved window locally, and if the persisted copy cannot be
decoded at startup it denies scheduled dispatch entirely until the next
successful sync rather than falling back to "always allowed".
<!-- docref: end -->

<!-- docref: begin src=contract/maintenance/window.go#entryAllows:11cc3934,contract/maintenance/window.go#Union:4a7b8d07 -->
The same discipline runs one level down: an entry whose clock range cannot be
parsed denies rather than permits. The union has the opposite bias by
construction — because an empty window means "always allowed", a device in *any*
group without a window is unrestricted. Windows narrow a device only when every
group reaching it agrees to narrow it.
<!-- docref: end -->

<!-- docref: begin src=agent/internal/scheduler/scheduler.go#Scheduler.runDue:e3bd1024 -->
**A deferred run is silent.** When the window is closed the agent simply does not
start the manifest — it does not report a skipped execution, and control sees
nothing at all until the window opens and the work actually runs. Do not look
for deferred work in the execution list; it is not there.
<!-- docref: end -->

Note also what the window does **not** gate. It applies to scheduled manifest
execution. Live operations — remote queries, log fetches, inventory collection,
terminal sessions — are not manifest deliveries and are unaffected by it.

> **Caveat, verified in code.** The dispatch request contract carries a
> `respect_maintenance_window` field. **No server code reads it.** The only thing
> that determines window exemption is the structural one-shot flag described
> below, and every explicit dispatch sets that flag. Setting the field changes
> nothing today.

---

## 5. One-shot dispatch

Assigned policy is recurring by nature. An operator pressing "run this now" is
not, and it is marked as a different thing rather than inferred:

<!-- docref: begin src=contract/proto/cadestro/v1/agent.proto#ManifestDelivery:88be620a -->
A one-shot delivery is flagged **structurally** on the manifest — not by action
type and not by the shape of its schedule, because assigned manifests may
legitimately carry an empty schedule and fall back to the agent's default drift
cadence. The agent executes such a delivery exactly once on durable receipt and
never reschedules it.
<!-- docref: end -->

<!-- docref: begin src=agent/internal/scheduler/scheduler.go#Scheduler.runDue:e3bd1024 -->
A one-shot delivery is also **exempt from the maintenance window**. An operator
asking for something now means now; assigned work keeps deferring until the
window opens. This is the only exemption — the flag does not bypass the receipt,
the audit trail, or any authorization check.
<!-- docref: end -->

---

## 6. Offline autonomy

An agent with no server connection is not an agent doing nothing.

<!-- docref: begin src=agent/internal/scheduler/scheduler.go#Scheduler.Start:2cf0bdf1,agent/internal/scheduler/scheduler.go#DefaultCheckInterval:67398dc3 -->
The scheduler loop reads exclusively from the agent's own local store, on a
one-minute tick plus an immediate wake when new work lands. Nothing in that loop
consults the network. A device that loses control keeps enforcing every manifest
it has already durably received, on schedule, indefinitely — including its
maintenance window, which is persisted locally for exactly this reason.
<!-- docref: end -->

<!-- docref: begin src=agent/cmd/cadestrod/runtime.go#sendScheduledResults:d20a8d78,agent/internal/executor/lps.go#Executor.setupLpsPasswords:8f33e833 -->
What stops while offline is precisely what needs the server. New assignments
cannot arrive, and results queue in a durable local outbox that drains on
reconnect. Two capabilities also refuse rather than proceed: password rotation
and LUKS passphrase management both require control to record the new secret, so
losing the connection makes them decline to rotate rather than change a
credential they cannot report. A rotated password nobody can look up is worse
than a password that is one cycle old.
<!-- docref: end -->

<!-- docref: begin src=agent/internal/scheduler/scheduler.go#Scheduler.executeManifest:c462e71c -->
Crash safety is handled by the same durable store. The agent marks an occurrence
`STARTED` before any non-idempotent side effect. If it dies between that mark and
the result, startup recovery reports `INDETERMINATE` rather than re-running the
work — a second attempt could double-apply, and reporting `FAILED` would be a
guess. Reboot actions are the one case with a real answer: the agent records the
kernel boot id alongside the started mark, and success is claimed only once a
later process observes a *different* boot id.
<!-- docref: end -->

---

## 7. Compliance is detection-only

Compliance in Cadestro **never remediates**. It is a separate posture from
enforcement, and the separation is enforced in code on both sides rather than by
convention.

<!-- docref: begin src=contract/proto/cadestro/v1/actions.proto#ShellParams:0ec72f48 -->
A shell action carries both a `detection_script` and an execution `script`, plus
an `is_compliance` flag. In ordinary enforcement the two work together:
detection runs first, exit 0 means compliant and the execution script is
skipped, non-zero runs the remediation and then re-runs detection to verify.
`is_compliance` is what turns that into a read-only check.
<!-- docref: end -->

<!-- docref: begin src=agent/internal/executor/executor.go#Executor.executeShellStreaming:a1d14e71 -->
On the agent, the compliance branch is evaluated **first** and returns before
the remediation path is reachable: it runs the detection script, reports the
finding, and never executes the action's script. It also fails closed — a
compliance action with an empty detection script is refused outright rather than
falling through to run its execution body, which was a real ordering bug and is
now the first thing the function checks.
<!-- docref: end -->

<!-- docref: begin src=server/internal/compliance/state.go#IsComplianceAction:29e66b3e,server/internal/compliance/state.go#ErrComplianceActionNeedsDetection:642c4e69 -->
On the server, the same predicate gates both attachment and result ingestion, so
the set of actions that can produce a finding is exactly the set attachment
accepts — the ingestion path cannot re-derive a looser rule. Attaching a
compliance rule whose action has no detection script fails closed with a named
error rather than enrolling a rule that would silently evaluate nothing.
<!-- docref: end -->

<!-- docref: begin src=contract/proto/cadestro/v1/common.proto#ComplianceStatus:60eadbe6 -->
A device's compliance status is one of `UNKNOWN`, `COMPLIANT`, `NON_COMPLIANT`,
or `IN_GRACE_PERIOD`. `UNKNOWN` is the zero value: a device that has not
reported is not counted as passing.
<!-- docref: end -->

<!-- docref: begin src=server/internal/execution/compliance.go#recordComplianceFinding:fcc3bad7,server/internal/execution/compliance.go#ruleStatus:65828b68 -->
Findings are ingested inside the execution result's own transaction, so the
compliance surface can never disagree with the execution evidence it was derived
from. Two rules govern what a finding means. First, **only a detection run that
actually completed counts as proof**: an agent reporting "compliant" alongside a
timeout or failure is recorded as non-compliant, because a check that did not
finish proves nothing. Second, a rule may carry a grace period, during which a
newly failing device reports `IN_GRACE_PERIOD` rather than `NON_COMPLIANT`;
the clock starts at the first failure and is cleared on recovery.
<!-- docref: end -->

<!-- docref: begin src=server/internal/store/compliance.go#RefreshDeviceCompliance:0fa64ea6 -->
"Never checked" and "checked and failed" stay distinguishable in both
directions. A device with no results at all stays `UNKNOWN` rather than being
counted as passing, and removing the rule that produced a failure clears the
device's status rather than leaving it stuck reporting a failure with no failing
check to point at.
<!-- docref: end -->

If you want a check that *also* fixes what it finds, that is an ordinary shell
action with both scripts set and `is_compliance` unset. The two modes are
deliberately not the same object.

---

## 8. Reading results

<!-- docref: begin src=contract/proto/cadestro/v1/common.proto#ExecutionStatus:36195ac8 -->
Execution status is richer than success/failure, and the distinctions carry
meaning you will want when triaging a fleet:

- `PENDING`, `RUNNING` — in flight.
- `SCHEDULED` — dispatched with a future run time, not yet due.
- `CANCELLED` — an operator cancelled it. Cancellation acts only while the
  execution is still `SCHEDULED` or `PENDING`; once the agent has picked it up,
  the cancel is a no-op and the row keeps its observed outcome.
- `SUCCESS`, `FAILED`, `TIMEOUT` — the ordinary terminal set.
- `SKIPPED` — a run that was deliberately not performed this time. In practice
  the agent emits this for occurrences abandoned after an earlier failure under
  `STOP`.
- `NOT_APPLICABLE` — the action is structurally inapplicable to this device: a
  `.deb` action on an rpm host, a Flatpak action with no Flatpak installed,
  security-only updates on a package manager with no security scoping. Terminal
  and non-error: nothing ran, fail-closed, and the machine-readable reason is in
  the result's error field. The difference from `SKIPPED` is that this is a
  property of the device/action pair, not of the moment.
- `INDETERMINATE` — the agent persisted `STARTED` before a non-idempotent effect
  and then crashed, so whether the effect landed is unknown. Terminal from the
  agent's side; resolving it is an operator decision.
<!-- docref: end -->

<!-- docref: begin src=contract/proto/cadestro/v1/agent.proto#ManifestResult:cadefa4b -->
Results arrive at two granularities: one `ActionResult` per occurrence, carrying
both the `delivery_id` and its `occurrence_id`, and one `ManifestResult` for the
delivery as a whole. Both identifiers are mandatory on a result, and control
keys the stored row on the pair — so a result replayed after a reconnect updates
the same row instead of creating a second one. The action id alone could not do
this, because duplicate authored occurrences legitimately produce several
results with the same action id.
<!-- docref: end -->

---

## Where to go next

- [Capability reference](capabilities.md) — every action type, its parameters,
  and the platform backends behind it.
- [Security model](security-model.md) — trust boundaries, identity, device PKI,
  and the audit guarantees under all of the above.
- [Enrollment](enrollment.md) — how a device gets a certificate and joins the
  stream that carries all of this.
