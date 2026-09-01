#!/usr/bin/env bash

set -euo pipefail

cd "$(dirname "$0")/.."
REPO_ROOT=$PWD
export GOWORK=off

echo "== gofmt"

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

if ! command -v staticcheck >/dev/null 2>&1; then
  echo "staticcheck is not installed — the gate cannot certify this tree" >&2
  exit 1
fi
echo "== staticcheck"
staticcheck ./...

echo "== go test"
go test -p 1 ./... -count=1

BUF="$REPO_ROOT/scripts/buf.sh"

echo "== buf lint"
(cd proto && "$BUF" lint)

echo "== buf format (drift)"
(cd proto && "$BUF" format --diff --exit-code)

echo "== generated-code drift"
make generate >/dev/null
if ! git diff --exit-code -I '^//\s*protoc\s+v[0-9]+\.[0-9]+\(\.[0-9]+\)\?' -- gen/; then
  echo "generated code in gen/ drifted from the proto sources — run 'make generate' and commit the result" >&2
  exit 1
fi

echo "== TypeScript typecheck"
npm run typecheck

echo "== contract gate green"
