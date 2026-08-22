#!/usr/bin/env bash
# The SDK's canonical verification gate — what CI runs, in one place.
#
# verify-stamp.sh prefers this over its generic Go gate. The generic gate runs
# `go build/vet/test ./...` from the repository root, which certifies whatever
# the workspace happens to stitch together rather than this module; and it
# knows nothing about `docref check`, so it would stamp the tree green without
# checking that the capability documentation still matches the code.
#
# GOWORK=off throughout: the SDK is consumed as a standalone module and must
# build as one. Inside this repository the sibling `replace` in go.mod resolves
# the contract module; outside it, the published module path does. A gate that
# only passes inside the workspace proves nothing for either.
#
# The contract half of the old combined gate — buf lint, buf format drift,
# generated-code drift, TypeScript — lives in contract/scripts/verify.sh, with
# the proto sources and the generator it belongs to.
set -euo pipefail

cd "$(dirname "$0")/.."
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
go test -p 1 ./... -count=1

if ! command -v docref >/dev/null 2>&1; then
  echo "docref is not installed — the gate cannot certify this tree" >&2
  exit 1
fi
echo "== docref check"
docref check

echo "== SDK gate green"
