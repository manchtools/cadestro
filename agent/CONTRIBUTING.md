# Contributing to the Cadestro Agent

The agent runs as root on managed devices and delegates OS work to the [SDK](../sdk) (`sys/*`, `pkg`) — the agent must not reimplement OS features the SDK provides. Shared idioms, branch naming, and commit conventions are the SDK's: see its [CONTRIBUTING](../sdk/CONTRIBUTING.md). Use an issue first; branch as `<prefix>/issue-<number>-<short-description>`.

## Test tiers

| Tier | Selector | Where it runs |
|---|---|---|
| Unit + arch | `go test -race ./...` (no tags) | host, every PR (`.github/workflows/agent.yml`) |
| Integration | `//go:build integration` files, functions named `TestIntegration_*` | 4-distro container matrix (`.github/workflows/agent-integration.yml`) |
| Privileged edge | same tag, functions named `TestEdgeCase_*` | privileged container lane |

Rules the CI enforces (self-discovering guards in `internal/archtest/`):
- Every integration-tagged test must live in a package the workflow tests **and** match a `-run` selector (`TestIntegration_*` / `TestEdgeCase_*`) — anything else never runs anywhere and the guard fails the build.
- Executor tests that touch the OS belong in the integration tier; unit tests use the seams (`FakeRunner`, `SetNowForTest`, backend fakes) — see `docs/container-test-strategy.md` for the full strategy and the dormant-test trap it prevents.

## Running the container lanes locally

```bash
# Distro matrix lane (debian; swap the Dockerfile suffix for fedora/opensuse/archlinux)
cd .. && docker build -f agent/test/Dockerfile.integration -t pm-agent-test .
docker run --rm pm-agent-test \
  go test -tags=integration -count=1 -timeout=10m ./agent/internal/executor/ -run Integration

# Privileged edge lane
docker run --rm --privileged pm-agent-test \
  go test -tags=integration -count=1 -timeout=10m ./agent/internal/executor/ -run EdgeCase
```

The build context is the **repository root**, and the image copies `contract/`, `sdk/`, and `agent/` into it: `agent/go.mod` replaces the contract and the SDK with those sibling directories, so an image built from `agent/` alone cannot resolve them. There is no branch-override mode — `internal/archtest` fails the build if one reappears, because an out-of-tree resolution means CI tested code this repository does not contain.

## Docs

`README.md` and `docs/` are docref-anchored: run `docker run --rm -v "$PWD:/repo" ghcr.io/manchtools/open-docref:v0.1.1 check` before pushing doc or code changes that touch anchored symbols; CI fails on drift. Update the prose *and* re-approve the claim — never delete an anchor to silence the check.
