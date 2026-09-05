# Project audit rules

Questions about how to run or configure the project request instructions, not
implementation. Supplied configuration values refine those instructions; they
do not authorize adding launchers, changing deployment files, or starting services.
Make those changes only when the operator explicitly asks for implementation.

When reducing backend scope, preserve the established UI and interaction design.
Adapt retained workflows to the current API; do not redesign the UI unless the
operator explicitly requests a visual redesign.

Interactive learning material must derive completion from a durable learner-produced answer or exercise result, never a self-certification checkbox.

## Class-wide coverage

For every repository-wide, module-wide, or polymorphic API audit, enumerate all
modules, backends, and implementations before drawing conclusions. Apply the
same parser-backed structural query and necessity-versus-complexity review to
every item, record an explicit verdict for every coverage cell, and report any
cell that could not be checked. User-supplied examples seed the search class;
they never define or limit its scope. Before recommending that a concept be
preserved or reintroduced, check repository history and current operator
rulings for that concept.

Every reported zero in an audit must be corroborated by a second independent
method, and the report must name both methods. A silent zero from one parser is
not evidence that a class is absent.

Before proposing a new authentication credential, trace the existing flow from
issuance through claim construction, request authentication, refresh,
revocation, and authority invalidation. Reuse the existing authorization path.
Any different permission-freshness or invalidation semantics require an
explicit operator ruling rather than being introduced as an implementation
detail.

## Agent and process isolation

Give every concurrent agent its own scratch namespace and require it to verify
that re-read artifacts belong to its assigned scope. Never terminate an
externally owned agent from a broad process listing: resolve the exact PID or
session ID explicitly authorized by the operator, and leave every unmatched
process running regardless of model name or age.

## Recorded operator rulings

Role and permission management are core control-plane capabilities; preserve
user-manageable roles, permission assignment, and enforcement across every
retained administrative RPC; do not replace them with hardcoded first-user or
single-admin gates when descoping product features.

Action definitions may persist the existing concrete action proto as a binary
blob under the scoped action-storage exception. The selected params oneof arm
is the sole action-kind authority: do not add a separate kind column, enum
discriminator, or type filter.

The existing ActionResult protobuf blob is the sole execution payload;
execution_results retains only relational metadata needed for identity,
device/action linking, and ordering; compliance classification comes only from
the linked action concrete oneof; no result-kind discriminator or flattened
status/error/output/detection/compliance columns.

ActionResult is an observation-only agent payload. The server derives
compliance from the linked concrete action and observed status and outputs; it
never trusts a client-supplied compliance or error outcome.

Administrative mutations each model one named operation with its own RPC and
response. Do not preserve broad CRUD update endpoints when rename,
description, configuration, enable/disable, or permission operations have
distinct authorization and behavior.

Lifecycle timestamps for mutable tables are owned by the database: every
insertion sets created_at and updated_at from the same CURRENT_TIMESTAMP, every
mutation advances updated_at in SQL, and application code does not supply those
lifecycle values. Caller-supplied domain timestamps remain distinct event facts.

Access tokens carry their effective permissions and authorize without a
per-request database lookup; role, permission, logout, and session-version
changes invalidate refresh-token generations immediately but already-issued
access tokens remain valid until their short expiry; increasing freshness later
requires an explicit operator ruling.

Ordinary authored actions are assigned and pulled during sync. Do not preserve
or reintroduce server-push dispatch for actions, action sets, definitions, or
groups, including durable one-shot delivery built only for that path. Push is
for genuinely live operations such as OSQuery, reboot, and terminal traffic.

Agent-stream messages and SDK methods must name desired-policy delivery
explicitly. Generic sync/state names hide that the agent requests its assigned
desired policy and must not be used for that operation. Names describe the state
the server wants the agent to achieve, not the transport shape used to deliver
it.

Goose migrations are the product's schema mechanism, and sqlc consumes that
canonical migration history for generated queries. The embedded Goose runner is
the automatic upgrade mechanism; never describe Cadestro as lacking migration
machinery. Before 1.0, unreleased transient history may be squashed into a reset
point, but an already released schema is immutable: every later schema change is
a new ordered migration with tested upgrade and rollback behavior. Do not
replace product migrations with a runtime baseline schema file.

API tokens authenticate automation agents as an existing OIDC user's identity.
Do not introduce a separate service-account principal, token-owned permissions,
or a parallel automation identity model. An operator who needs a dedicated
automation identity creates that user in OIDC, signs in once, and issues its API
token. Device-agent enrollment remains registration-token bootstrap followed by
mTLS and is not part of this API-token model.

Default roles are ordinary seed data, not immutable system roles. Administrators
may update, delete, or revoke any role, including defaults, assigned roles, and
the last administrator role. Registration tokens are revoked by deletion only;
finite-use tokens are deleted on their final successful enrollment use.

OIDC/PKCE transaction state is held in an authenticated Secure HttpOnly cookie
set by the control origin. Configured cross-origin web deployments use
credentialed fetch plus exact-origin credentialed CORS; do not persist OIDC
transaction state in auth_states. JavaScript cannot set document.cookie for
another origin: the control response sends Set-Cookie, and browser fetch uses
credentials: 'include'.
