#!/usr/bin/env bash

set -euo pipefail

DEPLOY_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$DEPLOY_DIR"

./setup.sh
docker compose config --quiet
docker compose pull
docker compose up -d --wait --remove-orphans
docker compose ps
