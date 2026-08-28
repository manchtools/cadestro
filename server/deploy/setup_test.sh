#!/usr/bin/env bash

set -euo pipefail

DEPLOY_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FIXTURE="$(mktemp -d)"
trap 'rm -rf "$FIXTURE"' EXIT

cp -R "$DEPLOY_DIR/." "$FIXTURE/"
cat > "$FIXTURE/.env" <<'EOF'
CONTROL_DOMAIN=manage.example.test
AGENT_DOMAIN=agents.example.test
ACME_EMAIL=admin@example.test
IMAGE_TAG=latest
OIDC_ISSUER_URL=https://id.example.test
OIDC_CLIENT_ID=cadestro
OIDC_CLIENT_SECRET=test-secret
EOF

bash "$FIXTURE/setup.sh" >/dev/null
bash "$FIXTURE/setup.sh" >/dev/null
docker compose --project-directory "$FIXTURE" --env-file "$FIXTURE/.env" -f "$FIXTURE/compose.yml" config --quiet

[[ "$(stat -c '%a' "$FIXTURE/certs/ca.key")" == 600 ]]
[[ "$(stat -c '%a' "$FIXTURE/secrets/encryption.key")" == 600 ]]
grep -Fxq 'CADESTRO_AGENT_LISTEN=0.0.0.0:8082' "$FIXTURE/config/control.env"
grep -Fxq 'CADESTRO_OIDC_ISSUER_URL=https://id.example.test' "$FIXTURE/config/control.env"
grep -Fxq 'CADESTRO_OIDC_CLIENT_ID=cadestro' "$FIXTURE/config/control.env"
grep -Fxq 'CADESTRO_OIDC_CLIENT_SECRET=test-secret' "$FIXTURE/config/control.env"
grep -Fq 'PathPrefix(`/cadestro.v1.ControlService`)' "$FIXTURE/traefik/dynamic/routes.yml"
grep -Fq 'address: control:8082' "$FIXTURE/traefik/dynamic/routes.yml"

if rg -i 'terminal|osquery|inventory|backup|artifact|proxyprotocol|scim' "$FIXTURE/config" "$FIXTURE/compose.yml" "$FIXTURE/traefik"; then
    exit 1
fi
