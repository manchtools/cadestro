# Contributing to Cadestro Agent

## Prerequisites

- Go 1.25+
- A Linux system for testing (the agent manages Linux devices; integration
  tests exercise real package managers and system services)

## Getting started

The agent builds standalone, resolving the contract and the SDK from their
sibling directories in this repository through the `replace` directives in
`agent/go.mod`:

```bash
go build ./cmd/cadestrod
go test ./...
```

Coordinated changes across modules need no extra setup: the repository root
carries a `go.work` that already stitches `agent`, `contract`, `sdk`, and
`server` together. Run the module's own gate with `GOWORK=off` to confirm it
still builds as a standalone module, which is what CI certifies.

## Workflow

1. Create a branch from `main`.
2. Make your changes with conventional commit messages:
   - `feat:` new feature
   - `fix:` bug fix
   - `chore:` maintenance
   - `docs:` documentation
   - `refactor:` code restructuring
   - `perf:` performance improvement
   - `test:` test additions/changes
3. Open a pull request. CodeRabbit reviews automatically.
4. Ensure CI passes before requesting review.

## Code style

- Follow existing patterns in the codebase.
- Always handle errors — never silently ignore them.
- Bug fixes come with a regression test that fails on the buggy version.

## Guardrails (architectural fitness functions)

`internal/archtest/` holds build-failing invariant tests that run in the
normal `go test ./...` path. Among what they enforce: string-literal SQL with
`?` placeholders only, constant-time comparison of secrets and signatures, no
unabstracted `time.Now()` in runtime code, no `context.Background()` in
request paths, no `math/rand` for anything cryptographic, no proto secret
fields reaching log or error sinks, no direct OS I/O in sensitive paths, and
no return of the abolished pre-rewrite agent runtime. Read the test files in
that package for the full, current list — the tests are the authority, not
this summary.

Each guard ships a documented allowlist for genuine exceptions. **Prefer
fixing the code over adding an allowlist entry.**

## License

By contributing, you agree that your contributions will be licensed under the
repository's [GPL-3.0](../LICENSE) license.
