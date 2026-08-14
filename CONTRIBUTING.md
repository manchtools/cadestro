# Contributing to Cadestro

## Building and testing

Every Go module builds and tests standalone. The repository root carries a
`go.work` so an editor can resolve all four at once, but **`GOWORK=off` is how
you verify**: a module that only compiles inside the workspace has an
undeclared dependency, and that is what CI certifies against.

| Module | Gate |
|---|---|
| `contract/` | `cd contract && ./scripts/verify.sh` |
| `sdk/` | `cd sdk && ./scripts/verify.sh` |
| `agent/` | `cd agent && GOWORK=off go build ./... && GOWORK=off go vet ./... && GOWORK=off go test ./...` |
| `server/` | `cd server && GOWORK=off go build ./... && GOWORK=off go vet ./... && GOWORK=off go test -timeout 30m ./...` |
| `web/` | `cd web && npm ci && npm run check && npm test` |

The contract and SDK gates are scripts because they run more than Go: the
contract's also lints the protobuf, checks buf formatting, asserts the
generated code has not drifted from its sources, and typechecks and tests the
TypeScript half.

Order matters for `web`: `npm run check` compiles the paraglide messages and
runs `svelte-kit sync`, and the test run needs both to have happened. Its
browser project drives a real Chromium — install it once with
`npx playwright install --with-deps chromium`.

The deployment is shell and its tests are shell, so `go test` does not reach
them:

```bash
cd server && bash deploy/setup_test.sh && bash deploy/install_test.sh && bash deploy/backup_test.sh
```

### Everything at once

```bash
./scripts/verify-all.sh
```

Runs every gate above plus the root structural check, **strictly one at a
time** and failing at the first non-zero exit. Sequential is not politeness:
several of these gates are heavy in the same build cache, module cache, and
disk at the same moment, and running them concurrently exhausts those and
reports a misleading build error instead of an honest test failure. It prints a
per-gate exit status at the end, so a gate that never ran is distinguishable
from one that passed.

`./scripts/verify.sh` on its own is the fast structural check — required root
files, the licensing map, and the predecessor-name guard. Run it while working;
run `verify-all.sh` before handing work over.

### Regenerating

Never edit generated code by hand.

```bash
cd contract && make generate     # protobuf -> gen/go and gen/ts
cd server   && make sqlc-generate # SQL queries -> internal/store/generated
```

Both are gated: CI regenerates and fails if the committed output differs.

## Documentation

The product documentation lives in [`docs/`](docs/) and is written **from the
code in this repository**, not from any predecessor corpus. It is one tree, not
one per module, because the pages cut across modules: enrollment is `agent/` and
`server/` together, the policy model is `contract/` plus both sides that
implement it, and the capability reference is `agent/internal/executor/` plus
the `sdk/sys/` backends underneath it.

Every behavioural statement in it is anchored with
[docref](https://github.com/manchtools/open-docref) to the symbol or region that
proves it, so a change to the code fails the docs that describe it:

```bash
docref check           # repository root: README.md + docs/ + agent/
cd server && docref check
cd sdk    && docref check
```

Three roots, one per `docref.toml`; `.github/workflows/docref.yml` runs all
three. Write the anchor as you write the sentence:

```bash
docref claim 'server/internal/ca/ca.go#CA.IssueCertificateFromCSR'   # a symbol
docref claim 'server/deploy/setup.sh#@archive-isolation'             # a region
```

That prints a paste-ready `<!-- docref: begin … -->` / `<!-- docref: end -->`
block with the hash already computed — never type or repair a hash by hand. When
a prose claim goes stale because the code moved, read the change and then
`docref approve <path>`; when only a materialized snippet moved, `docref
refresh` is mechanical. An un-anchored claim about behaviour is treated the same
way as an untested code path.

Module-local prose (`agent/README.md`, `server/deploy/QUICKSTART.md`,
`sdk/docs/`) stays where it is and stays anchored: it documents the module to
someone working *inside* it. `docs/` documents the product to someone deploying
and running it.

## Continuous integration

Workflows live in `.github/workflows/` at the repository root — GitHub honours
them nowhere else, so there is no module-local CI. Each module's workflow is
filtered to that module's paths **plus the modules it compiles against**, so a
contract change runs the SDK, agent, server, and web jobs too.

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
3. Open a pull request. Automated review runs on every PR.
4. Ensure CI passes before requesting review.

## Ground rules

- Every factual claim in documentation must be derivable from the code in this
  repository.
- Bug fixes come with a regression test that fails on the buggy version.
- Never edit generated code by hand; regenerate through the pinned tooling.
- Each Go module builds standalone (`GOWORK=off`); CI enforces it, because a
  module that only compiles inside the workspace has an undeclared dependency.

## Licensing of contributions

Each module carries its own license — see [LICENSING.md](LICENSING.md). By
contributing, you agree that your contributions are licensed under the license
of the module they touch.

## Security

Do not report vulnerabilities through public issues — see
[SECURITY.md](SECURITY.md).
