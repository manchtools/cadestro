#!/usr/bin/env bash

set -euo pipefail

REPO_ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
BUF="$REPO_ROOT/node_modules/.bin/buf"

if [ ! -x "$BUF" ]; then
  echo "buf is not installed at $BUF" >&2
  echo "run 'npm ci' in $REPO_ROOT — this repo runs ONLY the lockfile-pinned buf;" >&2
  echo "there is no PATH fallback and nothing is fetched at run time." >&2
  exit 1
fi

exec "$BUF" "$@"
