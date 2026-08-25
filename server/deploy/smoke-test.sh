#!/usr/bin/env bash

set -euo pipefail

SOURCE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WORK_DIR="$(mktemp -d)"
PROJECT_NAME="cadestro-smoke-$$"
PUBLISHED_IMAGE_TAG="${IMAGE_TAG:-}"
REQUESTED_IMAGE_TAG="${PUBLISHED_IMAGE_TAG:-smoke-$$}"
CONTROL_IMAGE="ghcr.io/manchtools/cadestro:$REQUESTED_IMAGE_TAG"
WEB_IMAGE="ghcr.io/manchtools/cadestro-web:$REQUESTED_IMAGE_TAG"
CONTROL_SOURCE_IMAGE="${CONTROL_SOURCE_IMAGE:-$CONTROL_IMAGE}"
WEB_SOURCE_IMAGE="${WEB_SOURCE_IMAGE:-ghcr.io/manchtools/cadestro-web:latest}"
BUILT_IMAGE=""
ALIASED_IMAGES=()





export IMAGE_TAG="$REQUESTED_IMAGE_TAG"

compose() {
    docker compose --project-directory "$WORK_DIR" --project-name "$PROJECT_NAME" \
        --env-file "$WORK_DIR/.env" "$@"
}




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
    for image in "${ALIASED_IMAGES[@]}"; do
        docker image rm "$image" >/dev/null 2>&1 || true
    done
    rm -rf "$WORK_DIR"
    exit "$status"
}
trap cleanup EXIT




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
        -t "$CONTROL_SOURCE_IMAGE" "$WORK_DIR"
    BUILT_IMAGE="$CONTROL_SOURCE_IMAGE"
fi
if [[ "$CONTROL_SOURCE_IMAGE" != "$CONTROL_IMAGE" ]]; then
    docker pull "$CONTROL_SOURCE_IMAGE"
    docker tag "$CONTROL_SOURCE_IMAGE" "$CONTROL_IMAGE"
    ALIASED_IMAGES+=("$CONTROL_IMAGE")
fi
if [[ "$WEB_SOURCE_IMAGE" != "$WEB_IMAGE" ]]; then
    docker pull "$WEB_SOURCE_IMAGE"
    docker tag "$WEB_SOURCE_IMAGE" "$WEB_IMAGE"
    ALIASED_IMAGES+=("$WEB_IMAGE")
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

compose up -d --wait

for service in "${services[@]}"; do
    compose ps --format '{{.Service}} {{.Health}}' "$service" | grep -Fq "$service healthy" || {
        printf '%s did not become healthy\n' "$service" >&2
        exit 1
    }
done

compose exec -T traefik traefik healthcheck --ping
compose exec -T control wget --no-check-certificate -q --spider https://127.0.0.1:8081/ready
compose exec -T web wget -q --spider http://127.0.0.1:3000/

schema_version="$(compose exec -T control sqlite3 /var/lib/cadestro/state/control.db \
    'SELECT COALESCE((SELECT version_id FROM goose_db_version WHERE is_applied ORDER BY id DESC LIMIT 1), 0);')"
[[ "$schema_version" =~ ^[1-9][0-9]*$ ]] || { printf 'SQLite Goose migration probe failed\n' >&2; exit 1; }
fts="$(compose exec -T control sqlite3 /var/lib/cadestro/state/control.db \
    "SELECT count(*) FROM sqlite_schema WHERE name = 'search_fts';")"
[[ "$fts" == 1 ]] || { printf 'SQLite FTS5 probe failed\n' >&2; exit 1; }

COMPOSE_PROJECT_NAME="$PROJECT_NAME" bash ./backup.sh >/dev/null
