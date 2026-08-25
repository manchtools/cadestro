#!/usr/bin/env bash

set -euo pipefail

SOURCE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WORK_DIR="$(mktemp -d)"
PROJECT_NAME="cadestro-smoke-$$"
PUBLISHED_IMAGE_TAG="${IMAGE_TAG:-}"
REQUESTED_IMAGE_TAG="${PUBLISHED_IMAGE_TAG:-smoke-$$}"
CONTROL_IMAGE="ghcr.io/manchtools/cadestro:$REQUESTED_IMAGE_TAG"
BUILT_IMAGE=""

# Compose substitutes from the process environment before any env file, and CI
# exports IMAGE_TAG as an empty string, which resolves compose.yml's
# ${IMAGE_TAG:-latest} to the stale published image instead of this run's tag.
# Export the tag this run actually uses so environment and .env agree.
export IMAGE_TAG="$REQUESTED_IMAGE_TAG"

compose() {
    docker compose --project-directory "$WORK_DIR" --project-name "$PROJECT_NAME" \
        --env-file "$WORK_DIR/.env" "$@"
}

# Control and backup.sh write into the state directories as root,
# and the host user running this script cannot unlink what they leave in the
# directories root creates underneath. Empty those from a container instead.
remove_root_owned_content() {
    local directory="$1"
    [[ -d "$directory" ]] || return 0
    docker run --rm --memory=6g --cpus=4 -v "$directory:/target" docker.io/library/alpine:3.23 \
        find /target -mindepth 1 -delete >/dev/null 2>&1 || true
}

cleanup() {
    local status=$?
    trap - EXIT
    if [[ $status -ne 0 ]]; then
        compose ps >&2 || true
        compose logs --no-color >&2 || true
    fi
    compose down --remove-orphans >/dev/null 2>&1 || true
    remove_root_owned_content "$WORK_DIR/data/control"
    if [[ -n "$BUILT_IMAGE" ]]; then
        docker image rm "$BUILT_IMAGE" >/dev/null 2>&1 || true
    fi
    rm -rf "$WORK_DIR"
    exit "$status"
}
trap cleanup EXIT

# The value of one variable in the rendered configuration. A name rendered zero
# times, or twice, fails here rather than yielding an empty string the caller
# would go on to compare.
control_env_value() {
    local name="$1" matches value
    matches="$(grep -c "^$name=" "$WORK_DIR/config/control.env" || true)"
    [[ "$matches" == 1 ]] || {
        printf '%s is set %s times in the rendered control.env, want once\n' "$name" "$matches" >&2
        return 1
    }
    value="$(sed -n "s|^$name=||p" "$WORK_DIR/config/control.env")"
    [[ -n "$value" ]] || {
        printf '%s is empty in the rendered control.env\n' "$name" >&2
        return 1
    }
    printf '%s\n' "$value"
}

cp -R "$SOURCE_DIR/." "$WORK_DIR/"
if [[ -z "$PUBLISHED_IMAGE_TAG" ]]; then
    CGO_ENABLED=0 go -C "$SOURCE_DIR/.." build -o "$WORK_DIR/cadestro" ./cmd/cadestro
    docker build --build-arg BINARY=cadestro -f "$SOURCE_DIR/Containerfile.control" \
        -t "$CONTROL_IMAGE" "$WORK_DIR"
    BUILT_IMAGE="$CONTROL_IMAGE"
fi
cat > "$WORK_DIR/.env" <<EOF
CONTROL_DOMAIN=manage.example.test
AGENT_DOMAIN=agents.example.test
ACME_EMAIL=admin@example.test
IMAGE_TAG=$REQUESTED_IMAGE_TAG
EOF

cd "$WORK_DIR"
bash ./setup.sh >/dev/null

mapfile -t services < <(compose config --services | sort)
[[ "${services[*]}" == "control traefik web" ]] || {
    printf 'unexpected deployment services: %s\n' "${services[*]}" >&2
    exit 1
}
if compose config | grep -q '/var/run/docker.sock'; then
    printf 'Traefik must not mount the container-engine socket\n' >&2
    exit 1
fi

compose up -d --wait control

schema_version="$(compose exec -T control sqlite3 /var/lib/cadestro/state/control.db 'PRAGMA user_version;')"
[[ "$schema_version" == 1 ]] || { printf 'SQLite schema probe failed\n' >&2; exit 1; }
fts="$(compose exec -T control sqlite3 /var/lib/cadestro/state/control.db \
    "SELECT count(*) FROM sqlite_schema WHERE name = 'search_fts';")"
[[ "$fts" == 1 ]] || { printf 'SQLite FTS5 probe failed\n' >&2; exit 1; }

COMPOSE_PROJECT_NAME="$PROJECT_NAME" bash ./backup.sh >/dev/null
