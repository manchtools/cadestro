# Project audit rules

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

Access tokens carry their effective permissions and authorize without a
per-request database lookup; role, permission, logout, and session-version
changes invalidate refresh-token generations immediately but already-issued
access tokens remain valid until their short expiry; increasing freshness later
requires an explicit operator ruling.

Ordinary authored actions are assigned and pulled during sync. Do not preserve
or reintroduce server-push dispatch for actions, action sets, definitions, or
groups, including durable one-shot delivery built only for that path. Push is
for genuinely live operations such as OSQuery, reboot, and terminal traffic.

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

Action definitions may persist the existing concrete action proto message as a binary blob with the action type stored separately; do not create a parallel storage proto. This is a scoped exception for action definitions and does not repeal the general API/storage separation rule.
