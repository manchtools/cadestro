# Cadestro

Cadestro is a small Linux fleet-management core for administrators:

- device enrollment with one-time registration tokens;
- an outbound, long-lived mTLS bidirectional stream;
- durable desired actions pulled by agents and executed while offline;
- native package installation, removal, and full-system updates;
- root, non-interactive shell actions as the escape hatch;
- detection-only shell compliance;
- static device groups, assignments, results, and audit events;
- OIDC login for administrators.

That is the product boundary. Cadestro does not currently provide a remote
terminal, OSQuery, inventory, log collection, self-service software, SCIM,
API tokens, user management, action sets, dynamic groups, or additional action
families.

## Layout

| Path | Purpose |
|---|---|
| `contract/` | Protobuf and Connect contract |
| `sdk/` | Reusable package, command, certificate, and agent support code |
| `agent/` | Root device agent with durable scheduling and result outbox |
| `server/` | SQLite control plane, OIDC, enrollment, sync, and audit |
| `web/` | Small Svelte administration console |

## Development

```bash
./scripts/verify-all.sh
```

All Go module gates run with `GOWORK=off`. Protobuf and sqlc output is
generated from its source and must not be edited directly.

The complete pre-descope implementation is preserved on
`archive/pre-core-20260828`.

## Deployment

The reference deployment is under `server/deploy/`.

## License

Licensing differs by module; see [LICENSING.md](LICENSING.md).
