# Contributing to the Cadestro server

## Build and test

```bash
go build ./cmd/cadestro
go test -p 1 ./...
```

Tests run against real SQLite database files, one isolated file per test —
there are no database mocks. The full gate is `scripts/verify.sh` (formatting,
vet, static analysis, the complete test suite, and docref).

## Generated code

Regenerate sqlc output only through the pinned commands:

```bash
make sqlc-generate
make sqlc-check
```

Never edit generated files by hand; CI verifies zero drift between the SQL
sources and the committed output.

## Verification expectations

Changes at a trust boundary must cover malformed, unauthenticated,
unauthorized, cross-owner, replay, cancellation, and persistence-failure
paths. State changes must roll back when audit persistence fails. Secret
values must not reach logs, errors, traces, audit payloads, or diagnostics.

Bug fixes require a regression test that fails on the buggy version. Scoped
non-owner access returns NotFound, never PermissionDenied. IDs are ULIDs.
Validation runs before authentication; authorization happens at the handler.

## Documentation

Prose about code is anchored with [docref](deploy/QUICKSTART.md) claims;
`docref check` must pass, and hashes are generated with `docref claim`, never
typed by hand. Anchor new behavioral claims when you write them, not as a
cleanup pass.

## Licensing of contributions

By contributing, you agree that your contributions are licensed under the
repository's [AGPL-3.0](LICENSE) license.

## Repository layout

- `cmd/cadestro/` — the `cadestro` server executable
- `internal/core/` — OIDC, administration RPCs, enrollment, and agent sync
- `internal/ca/` — device PKI
- `internal/crypto/` — encrypted OIDC client secrets
- `internal/store/` — Goose migrations, sqlc queries, and SQLite access
- `deploy/` — reference deployment
