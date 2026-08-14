#!/usr/bin/env bash
# The contract's canonical verification gate — what CI runs, in one place.
#
# GOWORK=off throughout: the contract is a leaf module that anything speaking
# this protocol resolves on its own, and that is how it must build.
#
# No `buf breaking`. CI does not run it either — V1 is the only version and the
# project takes clean breaks rather than compat shims, so a breaking-change gate
# would fail by design on every intentional contract change.
set -euo pipefail

cd "$(dirname "$0")/.."
REPO_ROOT=$PWD
export GOWORK=off

echo "== gofmt"
# No `2>/dev/null || true`: that swallows gofmt FAILING (unparseable file, bad
# path) and reports an empty violation list, so the check passes precisely when
# it could not run.
unfmt=$(gofmt -l .)
if [ -n "$unfmt" ]; then
  echo "gofmt violations:" >&2
  echo "$unfmt" >&2
  exit 1
fi

echo "== go build"
go build ./...

echo "== go vet"
go vet ./...

# Fail closed on a MISSING tool. Skipping it and reporting green is the exact
# shape this gate exists to prevent: a pass that means "not checked".
if ! command -v staticcheck >/dev/null 2>&1; then
  echo "staticcheck is not installed — the gate cannot certify this tree" >&2
  exit 1
fi
echo "== staticcheck"
staticcheck ./...

echo "== go test"
go test ./... -count=1

# buf runs from proto/ — proto/buf.yaml is the module root, so
# `import "powermanage/v1/common.proto"` only resolves from there. CI sets
# working-directory: proto for exactly this reason.
#
# The binary is resolved by scripts/buf.sh, which is fail-closed to the single
# lockfile-pinned install at this module root. It is a separate script rather
# than a function here because `make generate` below ALSO shells out to buf:
# with the resolution inlined in this file, the drift check would regenerate
# gen/ts with whatever `npx` picked while these two steps linted with the
# pinned copy, and the gate would certify a tree no single buf ever produced.
#
# There is deliberately no fallback to a PATH `buf`. That is not a fallback, it
# is an unpinned execution: `command -v buf` can resolve a go-installed binary
# with no relation to package-lock.json, so a missing `npm ci` would turn "the
# gate ran the pinned buf" into "the gate ran some buf" with no signal either
# way.
BUF="$REPO_ROOT/scripts/buf.sh"

echo "== buf lint"
(cd proto && "$BUF" lint)

echo "== buf format (drift)"
(cd proto && "$BUF" format --diff --exit-code)

# Generated-code drift. Without this the gate passes on a tree whose .proto and
# gen/ disagree: buf lints the SOURCE while the contract test reads the stale
# generated descriptor, so an RPC can be added or removed and both halves report
# green while contradicting each other.
#
# The `// protoc vX.Y.Z` comment is excluded exactly as CI excludes it — it flips
# with every protoc release and is not semantic.
echo "== generated-code drift"
make generate >/dev/null
if ! git diff --exit-code -I '^//\s*protoc\s+v[0-9]+\.[0-9]+\(\.[0-9]+\)\?' -- gen/; then
  echo "generated code in gen/ drifted from the proto sources — run 'make generate' and commit the result" >&2
  exit 1
fi

# The TypeScript half of the contract. gen/ts ships to consumers in the npm
# release, so a gate that only builds Go certifies half an artifact.
echo "== TypeScript typecheck"
npm run typecheck

echo "== TypeScript tests"
npm test

echo "== contract gate green"
