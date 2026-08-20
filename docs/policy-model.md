# The policy model

Cadestro manages devices by compiling authored actions into manifests. A
manifest is a flat, ordered list of action occurrences with one schedule and a
failure policy. The server stores the current assignment and the agent pulls
that state during authenticated synchronization.

## 1. Desired state

Actions describe the state a device should have: `PRESENT` or `ABSENT` for
stateful capabilities such as packages, files, services, users, and groups.
Re-running a manifest is safe and converges the device toward that state.

Schedules use either a standard five-field cron expression or an interval. A
manifest without a schedule uses the agent's eight-hour drift-prevention
cadence. Invalid cron falls back to that cadence and is logged.

## 2. Authoring and assignment

Actions can be grouped into action sets and definitions. Assignments attach an
action, action set, definition, or compliance policy to a device, device group,
user, or user group. Compilation resolves those sources into one flat manifest
in deterministic order; the agent never resolves the authoring tree.

Creating or changing an assignment changes desired state. It does not open a
second transport path or trigger work on the device immediately. The next
authenticated device sync returns the current assignment snapshot.

## 3. Pull synchronization

The agent maintains one outbound bidirectional mTLS stream. Synchronization
returns the device's current assigned manifests and any durable one-shot work
already created by an explicit dispatch. The agent records received work in its
local store before scheduling it, deduplicates by delivery id and manifest
bytes, and queues results for the next connection when offline.

Assigned manifests remain available for recurring local execution without a
server connection. A one-shot manifest executes once and becomes terminal.
Live operations such as reboot, inventory, logs, queries, and terminal use
their typed stream messages and do not pass through manifest scheduling.

## 4. Maintenance windows

Maintenance windows are evaluated using the device's local wall clock. The
server resolves the union of windows that reach a device and the agent stores
that resolved value with its work. An empty window means always allowed; an
invalid persisted window fails closed until the next successful sync.

One-shot work is exempt from the window because an explicit operator request is
immediate. Authorization and audit checks still apply.

## 5. Offline autonomy

The scheduler reads only local durable state. It continues enforcing manifests
already received while disconnected, and result rows remain in a durable outbox
until synchronization succeeds. Work that requires control to persist a new
secret refuses safely while offline.

Before a non-idempotent effect, the agent records the occurrence as started.
After a crash, the result is `INDETERMINATE` rather than running the effect a
second time.

## 6. Compliance is detection-only

Compliance actions run a detection script and never execute remediation. A
normal shell action may both detect and remediate; setting `is_compliance`
selects the read-only path. An empty detection script is rejected.

The server accepts findings only from actions that are valid compliance checks,
and records the finding with the execution result. A device with no completed
check remains `UNKNOWN`.

## 7. Results

Each occurrence reports its action id, delivery id, occurrence id, status, and
error details. The manifest result summarizes the run. Results are keyed by
those durable identifiers, so reconnects and retries update the same execution
instead of creating duplicates.

See [Capability reference](capabilities.md) for action implementations,
[Security model](security-model.md) for trust and audit guarantees, and
[Enrollment](enrollment.md) for joining the authenticated stream.
