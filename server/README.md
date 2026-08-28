# Cadestro Control Plane

The control plane is a single Go process backed by SQLite. It serves a public
TLS API for administrators and enrollment, plus a separate mTLS listener for
agent streams.

Its supported responsibilities are OIDC administrator login, registration
tokens, device certificates, devices, static device groups, package/update/shell
actions, assignments, desired-state synchronization, execution results,
detection-only compliance, and audit events.

Agents initiate every network connection. Assigned actions are pulled during
sync and retained by the agent for offline execution; the server does not push
authored actions.

The schema is an ordered Goose migration history. Startup applies pending
migrations, and sqlc consumes the same migration source.

```bash
./scripts/verify.sh
```

See [deploy/QUICKSTART.md](deploy/QUICKSTART.md) for the reference deployment.
