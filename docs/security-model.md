# Security model

Human administration uses OIDC. Cadestro stores no administrator password and
does not implement desktop SSO or end-user self-service.

Device enrollment uses a registration token only for bootstrap. Steady-state
device authentication is mTLS with an active certificate serial checked by the
control plane.

The agent initiates outbound traffic and exposes no network listener. Its local
enrollment socket is owner-only.

Package and shell actions are privileged host changes. Inputs are validated at
the contract boundary, package operands are passed without shell
interpolation, and shell environment variables reject process-hijacking names.

SQLite enables foreign keys, WAL mode, a busy timeout, and explicit durability.
Goose migrations are the schema source of truth. Audit events record
administrator mutations and device results.
