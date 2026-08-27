# Contributing to the Cadestro SDK

The SDK is the reusable mechanism layer: system capabilities and shared crypto
helpers. It carries no product policy and no wire contract — the protocol lives
in [`../contract`](../contract).

## Workflow

Use a focused branch and small commits. For a behavior change:

1. State the acceptance behavior in a test.
2. Make the smallest implementation change that passes it.
3. Verify this module and every consumer affected by the change.

Consumers resolve this module from the sibling directory, so a change here is
compiled by the agent and server in the same commit. Change them together
instead of maintaining old and new paths in parallel.

## Layout

| Path | Purpose |
|------|---------|
| `sys/` | Device capability implementations |
| `pkg/` | Package-manager capabilities |
| `crypto/`, `cryptotest/` | Shared crypto helpers and their X.509 fixtures |
| `logging/` | Metadata-only logging helpers |
| `archtest/`, `adversary/` | Architectural and cross-capability guards |
| `docs/` | Capability and contributor documentation |

## Capability rules

- Take an injected `exec.Runner`; never reach for a process-global runner or
  backend selector. `archtest` fails the build on either.
- Accept `context.Context` first for I/O and subprocess work.
- Return wrapped, matchable errors; do not swallow failures or panic in library
  code.
- Validate deserialized and caller-controlled data before use.
- Keep secret values out of arguments, logs, errors, and formatted output.
- Do not expose an implementation selector until more than one implementation
  actually exists and is supported end to end.
- Prefer concrete standard-library or platform mechanisms over new abstraction
  layers and dependencies.
- Do not import anything else from this repository. The SDK is a leaf module;
  the leaf-purity guard in `archtest` enforces this with no exceptions.

## Verification

The canonical gate is:

```bash
./scripts/verify.sh
```

It runs the standalone build with `GOWORK=off`, plus static analysis, tests,
and docref. For a change consumers see, also run the agent and server suites.

See
[`docs/04-contributing/01-release-coordination.md`](docs/04-contributing/01-release-coordination.md)
for how the modules resolve each other and how the repository is tagged.
