#!/usr/bin/env bash

set -euo pipefail
umask 077

INSTALL_DIR="${INSTALL_DIR:-/opt/cadestro}"
RELEASE_TAG="${RELEASE_TAG:-}"
GITHUB_REPOSITORY="${GITHUB_REPOSITORY:-MANCHTOOLS/cadestro}"

fail() { printf '[ERROR] %s\n' "$*" >&2; exit 1; }
for command_name in curl tar docker openssl; do
    command -v "$command_name" >/dev/null 2>&1 || fail "$command_name is required"
done
docker compose version >/dev/null 2>&1 || fail "the Docker Compose plugin is required"









hostname_pattern='^([A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?\.)+[A-Za-z]{2,63}$'
check_control_domain() { [[ "$1" =~ $hostname_pattern && "$1" != manage.example.com ]]; }
check_agent_domain() { [[ "$1" =~ $hostname_pattern && "$1" != agents.example.com && "$1" != "$CONTROL_DOMAIN" ]]; }
check_acme_email() { [[ "$1" =~ ^[^[:space:]@]+@[^[:space:]@]+\.[^[:space:]@]+$ && "$1" != admin@example.com ]]; }
check_release_tag() { [[ -n "$1" ]]; }
check_acme_challenge() { [[ -z "$1" || "$1" == http01 || "$1" == dns01 ]]; }
check_dns_provider() { [[ "$1" =~ ^[a-z0-9-]+$ ]]; }

ask() {
    local question="$1" check="$2" hint="$3" answer
    while true; do
        read -r -p "$question " answer || fail "input ended at: $question"
        if "$check" "$answer"; then
            printf '%s\n' "$answer"
            return 0
        fi
        printf '[ERROR] %s\n' "$hint" >&2
    done
}

if [[ -t 0 && -t 2 ]]; then
    [[ -n "${CONTROL_DOMAIN:-}" ]] || CONTROL_DOMAIN="$(ask \
        'Browser/API domain (e.g. control.example.com):' check_control_domain \
        'a fully-qualified hostname (not the documentation example) is required')"
    [[ -n "${AGENT_DOMAIN:-}" ]] || AGENT_DOMAIN="$(ask \
        'Agent domain, different from the browser/API one (e.g. agents.example.com):' check_agent_domain \
        "a fully-qualified hostname other than $CONTROL_DOMAIN (and not the documentation example) is required")"
    [[ -n "${ACME_EMAIL:-}" ]] || ACME_EMAIL="$(ask \
        "Email for Let's Encrypt expiry notices:" check_acme_email \
        'a real email address is required')"
    [[ -n "${RELEASE_TAG:-}" ]] || RELEASE_TAG="$(ask \
        'Release to install (e.g. v2026.08.09-rc2):' check_release_tag \
        'name the release to install; there is no default')"
    if [[ -z "${ACME_CHALLENGE:-}" ]]; then
        printf 'How should the HTTPS certificate be obtained?\n' >&2
        printf "  http01 - Let's Encrypt reaches this host on port 80 (default)\n" >&2
        printf '  dns01  - ownership is proven through your DNS provider instead\n' >&2
        ACME_CHALLENGE="$(ask 'Certificate challenge [http01]:' check_acme_challenge \
            'answer http01, dns01, or leave empty for http01')"
        ACME_CHALLENGE="${ACME_CHALLENGE:-http01}"
    fi
    if [[ "$ACME_CHALLENGE" == dns01 && -z "${ACME_DNS_PROVIDER:-}" ]]; then
        ACME_DNS_PROVIDER="$(ask \
            'DNS provider code from https://go-acme.github.io/lego/dns/ (e.g. hetzner):' check_dns_provider \
            'a lowercase lego provider code is required')"
    fi
fi

[[ -n "${CONTROL_DOMAIN:-}" ]] || fail "set CONTROL_DOMAIN"
[[ -n "${AGENT_DOMAIN:-}" ]] || fail "set AGENT_DOMAIN"
[[ -n "${ACME_EMAIL:-}" ]] || fail "set ACME_EMAIL"






[[ -n "$RELEASE_TAG" ]] \
    || fail "set RELEASE_TAG to the release to install, e.g. RELEASE_TAG=v2026.08.09-rc2"




ACME_CHALLENGE="${ACME_CHALLENGE:-http01}"
[[ "$ACME_CHALLENGE" == http01 || "$ACME_CHALLENGE" == dns01 ]] \
    || fail "ACME_CHALLENGE must be http01 or dns01"
[[ "$ACME_CHALLENGE" != dns01 || -n "${ACME_DNS_PROVIDER:-}" ]] \
    || fail "ACME_DNS_PROVIDER is required when ACME_CHALLENGE=dns01"

existing_material="$(find "$INSTALL_DIR/certs" "$INSTALL_DIR/secrets" "$INSTALL_DIR/config" \
    -mindepth 1 -print -quit 2>/dev/null || true)"
[[ -z "$existing_material" ]] \
    || fail "$INSTALL_DIR is already installed ($existing_material exists); update it with deploy.sh instead"

temporary_directory="$(mktemp -d)"
trap 'rm -rf "$temporary_directory"' EXIT
archive="$temporary_directory/source.tar.gz"

tag_url="https://github.com/${GITHUB_REPOSITORY}/archive/refs/tags/${RELEASE_TAG}.tar.gz"
branch_url="https://github.com/${GITHUB_REPOSITORY}/archive/refs/heads/${RELEASE_TAG}.tar.gz"
if ! curl -fsSL "$tag_url" -o "$archive"; then
    curl -fsSL "$branch_url" -o "$archive" || fail "could not download $GITHUB_REPOSITORY@$RELEASE_TAG"
fi

mkdir -p "$temporary_directory/source" "$INSTALL_DIR"
tar -xzf "$archive" -C "$temporary_directory/source"
source_root="$(find "$temporary_directory/source" -mindepth 1 -maxdepth 1 -type d -print -quit)"
[[ -f "$source_root/deploy/compose.yml" ]] || fail "release does not contain the deployment tree"
cp -R "$source_root/deploy/." "$INSTALL_DIR/"





if ! cmp -s "${BASH_SOURCE[0]}" "$source_root/deploy/install.sh" 2>/dev/null; then
    printf 'WARNING: this install.sh differs from the one inside release %s.\n' "$RELEASE_TAG" >&2
    printf 'Options it offered may not be understood by that release; prefer a release\n' >&2
    printf 'that matches this script, or RELEASE_TAG=main for the current tree.\n' >&2
fi

print_dns_reminder() {
    printf 'DNS: both records must point at this host:\n' >&2
    printf '    %s  (browser/API, HTTPS)\n' "$CONTROL_DOMAIN" >&2
    printf '    %s  (agent mTLS)\n' "$AGENT_DOMAIN" >&2
}

image_tag=latest
if [[ "$RELEASE_TAG" == v* ]]; then
    image_tag="${RELEASE_TAG#v}"
fi
cat > "$INSTALL_DIR/.env" <<EOF
CONTROL_DOMAIN=$CONTROL_DOMAIN
AGENT_DOMAIN=$AGENT_DOMAIN
ACME_EMAIL=$ACME_EMAIL
ACME_CHALLENGE=$ACME_CHALLENGE
ACME_DNS_PROVIDER=${ACME_DNS_PROVIDER:-}
IMAGE_TAG=$image_tag
EOF
chmod 600 "$INSTALL_DIR/.env"

cd "$INSTALL_DIR"





if [[ "$ACME_CHALLENGE" == dns01 && ! -s config/traefik-dns.env ]]; then
    mkdir -p config
    [[ -f config/traefik-dns.env ]] || install -m 600 /dev/null config/traefik-dns.env
    printf 'ACTION REQUIRED: paste your DNS provider credential into\n' >&2
    printf '    %s/config/traefik-dns.env\n' "$INSTALL_DIR" >&2
    printf 'as one KEY=VALUE line for the %s lego provider' "$ACME_DNS_PROVIDER" >&2
    if [[ "$ACME_DNS_PROVIDER" == hetzner ]]; then



        printf ' (HETZNER_API_TOKEN=<Cloud Console API token>)' >&2
    fi
    printf ', then finish with:\n' >&2
    printf '    cd %q && ./setup.sh && ./deploy.sh\n' "$INSTALL_DIR" >&2
    printf 'The credential is deliberately not asked for by a prompt: it belongs only in that file.\n' >&2
    print_dns_reminder
    exit 1
fi

./setup.sh
docker compose pull
docker compose up -d --wait

print_dns_reminder
printf 'Cadestro is running. The administration UI is served at https://%s and needs\n' "$CONTROL_DOMAIN"
printf 'no configuration; it talks to control on that same origin.\n'
printf 'Create the one-time administrator setup URL with:\n'
printf '  cd %q && docker compose exec control cadestro bootstrap-admin\n' "$INSTALL_DIR"
