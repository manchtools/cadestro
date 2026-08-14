---
title: Release coordination
label: Releases
description: How the SDK reaches the agent and server — module wiring, the leaf rule, and tag conventions.
---

# Release coordination

Contract, SDK, agent, and control server are modules of one repository. This
document is how a change to `sdk/` reaches the code that consumes it, and how
the repository is versioned for consumers outside it.

## How the modules are wired

Consumers resolve the two leaf modules from the sibling directory:

```go
// agent/go.mod and server/go.mod
require (
    github.com/manchtools/cadestro/contract v0.0.0
    github.com/manchtools/cadestro/sdk v0.0.0
)

replace github.com/manchtools/cadestro/contract => ../contract

replace github.com/manchtools/cadestro/sdk => ../sdk
```

<!-- docref: begin src=go.mod#@module-path:d4d2037e -->
The `v0.0.0` is a placeholder: the `replace` directive resolves the module
from the sibling directory, so the version is never consulted. The module
path (`github.com/manchtools/cadestro/sdk`) is the path this module publishes
under, not a location anything fetches from during a build in this
repository.
<!-- docref: end -->

The root `go.work` lists all four Go modules, so an editor and a repo-wide
`go build ./...` see one workspace. Every module also builds with `GOWORK=off`
— that is what the relative `replace` directives are for, and what each
module's gate runs. A module that builds only inside the workspace is broken
for everyone consuming it from outside.

## A change to the SDK is compiled by its consumers in the same commit

Consumers resolve the SDK from the tree rather than from a recorded version,
so every commit compiles one SDK. Change the SDK and its consumers together:

```bash
go build ./agent/... ./contract/... ./sdk/... ./server/...
```

compiles all of them from the repository root, and each module's gate re-runs
the build standalone. When an SDK change breaks the agent, the agent fix
belongs in that same change.

The module patterns are spelled out because the repository root is not itself
a Go module: in workspace mode `./...` resolves against the current module, so
from the root it matches nothing and the go command says so rather than
quietly building less than you asked for.

## The leaf rule

`contract` and `sdk` import nothing else in this repository, and `sdk` is
otherwise free of the generated protobuf types. This is not a convention: the
architecture test in `archtest` asserts it, and fails both when a new in-repo
import appears and when a recorded exception goes stale.

The reason is licensing. `contract` and `sdk` are MIT so that implementing the
protocol, or embedding a capability, imposes no obligation; `agent` and
`server` are copyleft. Permissive leaves feeding copyleft consumers is the
safe direction, and the reverse would not be.

## Tags and GitHub Releases

Tags name a state of the whole repository, not of one module.

| Identity | Format | Used by |
|---|---|---|
| Go module tag | `v0.x.x` semver | anyone consuming `contract` or `sdk` from outside |
| Human-readable release label | `vYYYY.MM.XX` calendar date (e.g. `v2026.04.03`) | GitHub Releases UI, operator-facing docs |

### Why two conventions

Go modules applies **Semantic Import Versioning**: for major version ≥ 2, the
import path must carry a `/vN` suffix (e.g. `github.com/foo/bar/v2`).
Calendar-style tags like `v2026.x.x` would require renaming the import path to
`github.com/manchtools/cadestro/sdk/v2026`, which ripples into every `import`
statement and has to be redone every January. That is a high price for a
version-number aesthetic.

The repository sidesteps it by staying in the `v0.x.x` / `v1.x.x` range for Go
consumption while keeping calendar-dated GitHub Release names for humans. Both
coexist at the same commit.

### The pre-v1.0.0 contract

`contract` and `sdk` are on a `v0.x.x` line, which per semver means the public
API is not yet stable. **Minor bumps (`v0.1.0` → `v0.2.0`) may carry breaking
changes.** A move to `v1.x.x` is a deliberate decision to freeze the public
surface; don't tag `v1.0.0` until the API has settled, because once it is cut,
breaking changes become a coordinated `v2.x.x` move with a new import path.

## Anti-patterns

- **Don't delete old tags.** External consumers may still pin to them.
- **Don't tag `v1.0.0` prematurely.** Once the `v1.x.x` line is cut, the API
  is frozen; breaking changes require moving to `v2.x.x` and renaming the
  import path.
- **Don't tag `v2.x.x` without renaming the import path.** Go's Semantic
  Import Versioning requires a `/v2` suffix in the path, and skipping it
  produces invalid modules that nobody can consume.
- **Don't replace a relative `replace` with a version pin** to insulate a
  consumer from an in-tree change. The agent's architecture test fails on that
  deliberately: a consumer that resolves the SDK from anywhere but the sibling
  directory is compiling against code this repository does not contain.
- **Don't let a module build only inside the workspace.** Run its gate with
  `GOWORK=off`; that is how everyone outside this repository resolves it.
