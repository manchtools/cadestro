#!/bin/bash
# Deploys a new web version and pins the previous latest as a versioned service.
#
# Usage: ./deploy.sh <version>
#   e.g.: ./deploy.sh v2026.02.19
#
# Environment:
#   DEPLOY_DIR     — working directory (default: /opt/cadestro-web)
#   GHCR_TOKEN     — GHCR PAT for docker login (optional if already logged in)
#   GHCR_USER      — GHCR username (default: github)
#
# Host setup (one-time):
#   1. Install docker, docker compose, yq
#   2. Either run `docker login ghcr.io` manually, or set GHCR_TOKEN secret in CI
#   3. Create a deploy user with docker group access and SSH key
#   4. Ensure Traefik is running and routing PathPrefix(`/app`)
set -euo pipefail

NEW_VERSION="${1:?Usage: $0 <version>}"
DEPLOY_DIR="${DEPLOY_DIR:-/opt/cadestro-web}"
VERSIONS_FILE="${DEPLOY_DIR}/versions.conf"
COMPOSE_FILE="${DEPLOY_DIR}/docker-compose.yml"
IMAGE="ghcr.io/manchtools/cadestro-web"

mkdir -p "$DEPLOY_DIR"

# Login to GHCR if token provided
if [ -n "${GHCR_TOKEN:-}" ]; then
	echo "$GHCR_TOKEN" | docker login ghcr.io -u "${GHCR_USER:-github}" --password-stdin
fi

# --- Update version tracking ---
if [ -f "$VERSIONS_FILE" ]; then
	CURRENT_LATEST=$(head -1 "$VERSIONS_FILE")
	if [ "$CURRENT_LATEST" = "$NEW_VERSION" ]; then
		echo "Already at ${NEW_VERSION}, re-pulling and restarting..."
	else
		# Prepend new version, keep old ones as pinned
		{ echo "$NEW_VERSION"; cat "$VERSIONS_FILE"; } > "${VERSIONS_FILE}.tmp"
		mv "${VERSIONS_FILE}.tmp" "$VERSIONS_FILE"
	fi
else
	echo "$NEW_VERSION" > "$VERSIONS_FILE"
fi

# --- Generate docker-compose.yml ---
LATEST=$(head -1 "$VERSIONS_FILE")

# Write header + latest service
cat > "$COMPOSE_FILE" << EOF
services:
  web-latest:
    image: ${IMAGE}:${LATEST}
    restart: unless-stopped
    labels:
EOF
# Traefik labels (printf avoids heredoc backtick issues)
printf '      - "traefik.enable=true"\n' >> "$COMPOSE_FILE"
printf '      - "traefik.http.routers.web-latest.rule=PathPrefix(`/app`)"\n' >> "$COMPOSE_FILE"
printf '      - "traefik.http.routers.web-latest.priority=50"\n' >> "$COMPOSE_FILE"
printf '      - "traefik.http.services.web-latest.loadbalancer.server.port=3000"\n' >> "$COMPOSE_FILE"

# Write pinned version services
if [ "$(wc -l < "$VERSIONS_FILE")" -gt 1 ]; then
	tail -n +2 "$VERSIONS_FILE" | while IFS= read -r version; do
		[ -z "$version" ] && continue
		service_name="web-$(echo "$version" | tr '.' '-')"
		# Cookie stores version without v prefix (e.g. 2026.02.15)
		cookie_version="${version#v}"
		escaped_cookie=$(echo "$cookie_version" | sed 's/\./\\./g')

		printf '\n  %s:\n' "$service_name" >> "$COMPOSE_FILE"
		printf '    image: %s:%s\n' "$IMAGE" "$version" >> "$COMPOSE_FILE"
		printf '    restart: unless-stopped\n' >> "$COMPOSE_FILE"
		printf '    labels:\n' >> "$COMPOSE_FILE"
		printf '      - "traefik.enable=true"\n' >> "$COMPOSE_FILE"
		printf '      - "traefik.http.routers.%s.rule=PathPrefix(`/app`) && HeadersRegexp(`Cookie`, `pm-version=%s`)"\n' "$service_name" "$escaped_cookie" >> "$COMPOSE_FILE"
		printf '      - "traefik.http.routers.%s.priority=100"\n' "$service_name" >> "$COMPOSE_FILE"
		printf '      - "traefik.http.services.%s.loadbalancer.server.port=3000"\n' "$service_name" >> "$COMPOSE_FILE"
	done
fi

echo ""
echo "Generated ${COMPOSE_FILE}:"
cat "$COMPOSE_FILE"
echo ""

# --- Pull and deploy ---
docker compose -f "$COMPOSE_FILE" pull
docker compose -f "$COMPOSE_FILE" up -d --remove-orphans

echo ""
echo "Deployed ${NEW_VERSION}. Active versions:"
cat "$VERSIONS_FILE"
