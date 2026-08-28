#!/usr/bin/env bash

set -euo pipefail
umask 077

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CERTS_DIR="$SCRIPT_DIR/certs"
CONFIG_DIR="$SCRIPT_DIR/config"
SECRETS_DIR="$SCRIPT_DIR/secrets"
DATA_DIR="$SCRIPT_DIR/data"

fail() {
    printf '[ERROR] %s\n' "$*" >&2
    exit 1
}

load_environment() {
    local file="$SCRIPT_DIR/.env" line name value
    [[ -f "$file" ]] || fail "copy .env.example to .env and configure it"
    while IFS= read -r line || [[ -n "$line" ]]; do
        [[ -n "$line" ]] || continue
        [[ "$line" != '#'* ]] || continue
        [[ "$line" =~ ^[A-Za-z_][A-Za-z0-9_]*= ]] || fail "invalid .env line: $line"
        name="${line%%=*}"
        value="${line#*=}"
        case "$name" in
            CONTROL_DOMAIN|AGENT_DOMAIN|ACME_EMAIL|IMAGE_TAG|OIDC_ISSUER_URL|OIDC_CLIENT_ID) ;;
            *) fail "unknown .env variable: $name" ;;
        esac
        printf -v "$name" '%s' "$value"
        export "$name"
    done < "$file"
}

validate_environment() {
    local hostname='^([A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?\.)+[A-Za-z]{2,63}$'
    [[ "${CONTROL_DOMAIN:-}" =~ $hostname ]] || fail "CONTROL_DOMAIN must be a fully-qualified hostname"
    [[ "${AGENT_DOMAIN:-}" =~ $hostname ]] || fail "AGENT_DOMAIN must be a fully-qualified hostname"
    [[ "$CONTROL_DOMAIN" != "$AGENT_DOMAIN" ]] || fail "CONTROL_DOMAIN and AGENT_DOMAIN must differ"
    [[ "${ACME_EMAIL:-}" =~ ^[^[:space:]@]+@[^[:space:]@]+\.[^[:space:]@]+$ ]] || fail "ACME_EMAIL is invalid"
    [[ "${OIDC_ISSUER_URL:-}" == https://* ]] || fail "OIDC_ISSUER_URL must use HTTPS"
    [[ -n "${OIDC_CLIENT_ID:-}" ]] || fail "OIDC_CLIENT_ID is required"
    [[ "$CONTROL_DOMAIN" != manage.example.com ]] || fail "replace the example CONTROL_DOMAIN"
    [[ "$AGENT_DOMAIN" != agents.example.com ]] || fail "replace the example AGENT_DOMAIN"
    [[ "$ACME_EMAIL" != admin@example.com ]] || fail "replace the example ACME_EMAIL"
}

validate_key_pair() {
    local certificate="$1" key="$2"
    openssl x509 -in "$certificate" -noout >/dev/null
    openssl pkey -in "$key" -noout >/dev/null
    cmp -s <(openssl x509 -in "$certificate" -pubkey -noout | openssl pkey -pubin -outform DER 2>/dev/null) <(openssl pkey -in "$key" -pubout -outform DER 2>/dev/null) || fail "$certificate and $key do not match"
}

ensure_ca() {
    if [[ -f "$CERTS_DIR/ca.crt" && -f "$CERTS_DIR/ca.key" ]]; then
        validate_key_pair "$CERTS_DIR/ca.crt" "$CERTS_DIR/ca.key"
        return
    fi
    [[ ! -e "$CERTS_DIR/ca.crt" && ! -e "$CERTS_DIR/ca.key" ]] || fail "CA certificate and key must both exist"
    openssl genpkey -algorithm Ed25519 -out "$CERTS_DIR/ca.key"
    openssl req -new -x509 -key "$CERTS_DIR/ca.key" -days 3650 -subj "/CN=Cadestro Internal CA/O=Cadestro" -addext "basicConstraints=critical,CA:TRUE" -addext "keyUsage=critical,keyCertSign,cRLSign" -out "$CERTS_DIR/ca.crt"
}

ensure_control_certificate() {
    if [[ -f "$CERTS_DIR/control.crt" && -f "$CERTS_DIR/control.key" ]]; then
        validate_key_pair "$CERTS_DIR/control.crt" "$CERTS_DIR/control.key"
        openssl x509 -in "$CERTS_DIR/control.crt" -checkhost "$AGENT_DOMAIN" -noout >/dev/null || fail "control certificate does not cover AGENT_DOMAIN"
        return
    fi
    [[ ! -e "$CERTS_DIR/control.crt" && ! -e "$CERTS_DIR/control.key" ]] || fail "control certificate and key must both exist"
    local csr="$CERTS_DIR/control.csr"
    openssl genpkey -algorithm Ed25519 -out "$CERTS_DIR/control.key"
    openssl req -new -key "$CERTS_DIR/control.key" -subj "/CN=$AGENT_DOMAIN/O=Cadestro" -out "$csr"
    openssl x509 -req -in "$csr" -CA "$CERTS_DIR/ca.crt" -CAkey "$CERTS_DIR/ca.key" -CAcreateserial -days 825 -extfile <(printf 'subjectAltName=DNS:%s,DNS:control,DNS:localhost\nextendedKeyUsage=serverAuth\nkeyUsage=digitalSignature\n' "$AGENT_DOMAIN") -out "$CERTS_DIR/control.crt"
    rm -f "$csr"
}

ensure_secrets() {
    [[ -f "$SECRETS_DIR/session-signing.pem" ]] || openssl genpkey -algorithm Ed25519 -out "$SECRETS_DIR/session-signing.pem"
    openssl pkey -in "$SECRETS_DIR/session-signing.pem" -noout >/dev/null || fail "session signing key is invalid"
}

write_config() {
    local lines=(
        'CADESTRO_PUBLIC_LISTEN=0.0.0.0:8081'
        'CADESTRO_AGENT_LISTEN=0.0.0.0:8082'
        "CADESTRO_PUBLIC_BASE_URL=https://$CONTROL_DOMAIN"
        "CADESTRO_AGENT_URL=https://$AGENT_DOMAIN"
        "CADESTRO_CORS_ORIGINS=https://$CONTROL_DOMAIN"
        'CADESTRO_LOG_LEVEL=info'
        'CADESTRO_LOG_FORMAT=json'
        'CADESTRO_CERTIFICATE_VALIDITY=8760h'
        'CADESTRO_HEARTBEAT_INTERVAL=30s'
        'CADESTRO_DATABASE_PATH=/var/lib/cadestro/control.db'
        'CADESTRO_CA_CERT_FILE=/run/certs/ca.crt'
        'CADESTRO_CA_KEY_FILE=/run/certs/ca.key'
        'CADESTRO_AGENT_TLS_CERT_FILE=/run/certs/control.crt'
        'CADESTRO_AGENT_TLS_KEY_FILE=/run/certs/control.key'
        'CADESTRO_PUBLIC_TLS_CERT_FILE=/run/certs/control.crt'
        'CADESTRO_PUBLIC_TLS_KEY_FILE=/run/certs/control.key'
        'CADESTRO_SESSION_SIGNING_KEY_FILE=/run/secrets/session-signing.pem'
        'CADESTRO_OIDC_NAME=Company SSO'
        'CADESTRO_OIDC_SLUG=sso'
        "CADESTRO_OIDC_ISSUER_URL=$OIDC_ISSUER_URL"
        "CADESTRO_OIDC_CLIENT_ID=$OIDC_CLIENT_ID"
        'CADESTRO_OIDC_SCOPES=openid,profile,email'
    )
    printf '%s\n' "${lines[@]}" > "$CONFIG_DIR/control.env"
    printf 'PUBLIC_CONTROL_URL=https://%s\n' "$CONTROL_DOMAIN" > "$CONFIG_DIR/web.env"
    printf 'TRAEFIK_CERTIFICATESRESOLVERS_LETSENCRYPT_ACME_HTTPCHALLENGE_ENTRYPOINT=web\n' > "$CONFIG_DIR/traefik.env"
}

main() {
    command -v openssl >/dev/null || fail "openssl is required"
    command -v cmp >/dev/null || fail "cmp is required"
    load_environment
    validate_environment
    mkdir -p "$CERTS_DIR" "$CONFIG_DIR" "$SECRETS_DIR" "$DATA_DIR/control" "$DATA_DIR/traefik"
    chmod 700 "$CERTS_DIR" "$CONFIG_DIR" "$SECRETS_DIR" "$DATA_DIR/control"
    touch "$DATA_DIR/traefik/acme.json"
    chmod 600 "$DATA_DIR/traefik/acme.json"
    ensure_ca
    ensure_control_certificate
    ensure_secrets
    write_config
    chmod 600 "$SCRIPT_DIR/.env" "$CERTS_DIR"/*.key "$SECRETS_DIR"/* "$CONFIG_DIR"/*.env
    chmod 644 "$CERTS_DIR"/*.crt
    printf 'Deployment material is ready. Run docker compose up -d --wait.\n'
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
    main "$@"
fi
