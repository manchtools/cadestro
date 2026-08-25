#!/usr/bin/env bash

set -euo pipefail
umask 077

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CERTS_DIR="$SCRIPT_DIR/certs"
CONFIG_DIR="$SCRIPT_DIR/config"
SECRETS_DIR="$SCRIPT_DIR/secrets"
DATA_DIR="$SCRIPT_DIR/data"

info() { printf '[INFO] %s\n' "$*"; }
fail() { printf '[ERROR] %s\n' "$*" >&2; exit 1; }

require_command() {
    command -v "$1" >/dev/null 2>&1 || fail "$1 is required"
}







load_environment() {
    local file="$SCRIPT_DIR/.env" line trimmed name value number=0
    [[ -f "$file" ]] || fail "copy .env.example to .env and set the two domains and ACME email"


    while IFS= read -r line || [[ -n "$line" ]]; do
        number=$((number + 1))
        trimmed="${line#"${line%%[![:space:]]*}"}"
        [[ -n "$trimmed" ]] || continue
        [[ "$trimmed" != '#'* ]] || continue
        [[ "$line" =~ ^[A-Za-z_][A-Za-z0-9_]*= ]] \
            || fail "$file line $number is not a KEY=VALUE assignment: $line"
        name="${line%%=*}"


        value="${line#*=}"



        [[ "$value" != [\"\']* ]] \
            || fail "$file line $number quotes its value; write $name=value without quotes"


        export "$name=$value"
    done < "$file"
}

validate_environment() {
    local hostname='^([A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?\.)+[A-Za-z]{2,63}$'
    [[ "${CONTROL_DOMAIN:-}" =~ $hostname ]] || fail "CONTROL_DOMAIN must be a fully-qualified hostname"
    [[ "${AGENT_DOMAIN:-}" =~ $hostname ]] || fail "AGENT_DOMAIN must be a fully-qualified hostname"
    [[ "$CONTROL_DOMAIN" != "$AGENT_DOMAIN" ]] || fail "CONTROL_DOMAIN and AGENT_DOMAIN must differ"
    [[ "${ACME_EMAIL:-}" =~ ^[^[:space:]@]+@[^[:space:]@]+\.[^[:space:]@]+$ ]] || fail "ACME_EMAIL is invalid"
    [[ "$CONTROL_DOMAIN" != manage.example.com && "$AGENT_DOMAIN" != agents.example.com ]] \
        || fail "replace the example hostnames in .env"
    [[ "$ACME_EMAIL" != admin@example.com ]] || fail "replace the example ACME email in .env"
    ACME_CHALLENGE="${ACME_CHALLENGE:-http01}"
    [[ "$ACME_CHALLENGE" == http01 || "$ACME_CHALLENGE" == dns01 ]] \
        || fail "ACME_CHALLENGE must be http01 or dns01"
    [[ "$ACME_CHALLENGE" != dns01 || -n "${ACME_DNS_PROVIDER:-}" ]] \
        || fail "ACME_DNS_PROVIDER is required when ACME_CHALLENGE=dns01"
}

require_pair() {
    local first="$1" second="$2" description="$3"
    if [[ -e "$first" || -e "$second" ]]; then
        [[ -f "$first" && -f "$second" ]] || fail "$description requires both $(basename "$first") and $(basename "$second")"
        return 0
    fi
    return 1
}

validate_key_pair() {
    local certificate="$1" key="$2" description="$3"
    openssl x509 -in "$certificate" -noout >/dev/null
    openssl pkey -in "$key" -noout >/dev/null
    cmp -s \
        <(openssl x509 -in "$certificate" -pubkey -noout | openssl pkey -pubin -outform DER 2>/dev/null) \
        <(openssl pkey -in "$key" -pubout -outform DER 2>/dev/null) \
        || fail "$description certificate and private key do not match"
}

# docref: begin generated-material









ensure_traefik_acme_config() {
    local credentials="$CONFIG_DIR/traefik-dns.env"
    if [[ "$ACME_CHALLENGE" == dns01 ]]; then
        [[ -f "$credentials" ]] || fail "$(printf '%s\n' \
            "$credentials does not exist; ACME_CHALLENGE=dns01 needs the credentials for" \
            "ACME_DNS_PROVIDER=${ACME_DNS_PROVIDER}, one KEY=VALUE per line, readable by nobody else:" \
            "" \
            "    install -m 600 /dev/null $credentials" \
            "" \
            "then write the provider's variable into it - for Hetzner DNS that is HETZNER_API_TOKEN" \
            "with a Cloud Console API token (HETZNER_API_KEY selects the legacy API shut down in May 2026).")"
        [[ -s "$credentials" ]] \
            || fail "$credentials is empty; write the ACME_DNS_PROVIDER credentials into it, one KEY=VALUE per line"


        [[ "$(stat -c '%a' "$credentials")" =~ ^[0-6]00$ ]] \
            || fail "$credentials must not be group/world accessible; chmod 600 it and rotate the credentials it holds"








        cat > "$CONFIG_DIR/traefik-acme.env" <<EOF
TRAEFIK_CERTIFICATESRESOLVERS_LETSENCRYPT_ACME_DNSCHALLENGE_PROVIDER=${ACME_DNS_PROVIDER}
TRAEFIK_CERTIFICATESRESOLVERS_LETSENCRYPT_ACME_DNSCHALLENGE_RESOLVERS=1.1.1.1:53,9.9.9.9:53
TRAEFIK_CERTIFICATESRESOLVERS_LETSENCRYPT_ACME_DNSCHALLENGE_PROPAGATION_DELAYBEFORECHECKS=60s
EOF
    else



        [[ -f "$credentials" ]] || : > "$credentials"
        chmod 600 "$credentials"
        cat > "$CONFIG_DIR/traefik-acme.env" <<'EOF'
TRAEFIK_CERTIFICATESRESOLVERS_LETSENCRYPT_ACME_HTTPCHALLENGE_ENTRYPOINT=web
EOF
    fi
    chmod 600 "$CONFIG_DIR/traefik-acme.env"
}

ensure_ca() {
    if require_pair "$CERTS_DIR/ca.crt" "$CERTS_DIR/ca.key" "certificate authority"; then
        validate_key_pair "$CERTS_DIR/ca.crt" "$CERTS_DIR/ca.key" "certificate authority"
        openssl verify -CAfile "$CERTS_DIR/ca.crt" "$CERTS_DIR/ca.crt" >/dev/null
        openssl x509 -in "$CERTS_DIR/ca.crt" -text -noout | grep -q 'CA:TRUE' \
            || fail "ca.crt is not a certificate authority"
        info "Using existing certificate authority"
        return
    fi

    info "Generating internal Ed25519 certificate authority"
    openssl genpkey -algorithm Ed25519 -out "$CERTS_DIR/ca.key"
    openssl req -new -x509 -key "$CERTS_DIR/ca.key" -days 3650 \
        -subj "/CN=Cadestro Internal CA/O=Cadestro" \
        -addext "basicConstraints=critical,CA:TRUE" \
        -addext "keyUsage=critical,keyCertSign,cRLSign" \
        -out "$CERTS_DIR/ca.crt"
}

ensure_certificate() {
    local name="$1" subject="$2" extensions="$3"
    local certificate="$CERTS_DIR/$name.crt" key="$CERTS_DIR/$name.key" csr="$CERTS_DIR/$name.csr"
    if require_pair "$certificate" "$key" "$name certificate"; then
        validate_key_pair "$certificate" "$key" "$name"
        openssl verify -CAfile "$CERTS_DIR/ca.crt" "$certificate" >/dev/null
        return
    fi

    info "Generating $name certificate"
    openssl genpkey -algorithm Ed25519 -out "$key"
    openssl req -new -key "$key" -subj "$subject" -out "$csr"
    openssl x509 -req -in "$csr" -CA "$CERTS_DIR/ca.crt" -CAkey "$CERTS_DIR/ca.key" \
        -CAcreateserial -days 825 -extfile <(printf '%b\n' "$extensions") -out "$certificate"
    rm -f "$csr"
    openssl verify -CAfile "$CERTS_DIR/ca.crt" "$certificate" >/dev/null
}







certificate_covers_host() {
    local certificate="$1" host="$2"
    [[ "$(openssl x509 -in "$certificate" -checkhost "$host" -noout 2>/dev/null)" \
        == "Hostname $host does match certificate" ]]
}

ensure_certificates() {
    ensure_certificate control "/CN=$AGENT_DOMAIN/O=Cadestro" \
        "subjectAltName=DNS:$AGENT_DOMAIN,DNS:control,DNS:localhost\nextendedKeyUsage=serverAuth\nkeyUsage=digitalSignature"
    certificate_covers_host "$CERTS_DIR/control.crt" "$AGENT_DOMAIN" \
        || fail "control.crt does not cover AGENT_DOMAIN; replace control.crt and control.key"
    certificate_covers_host "$CERTS_DIR/control.crt" control \
        || fail "control.crt does not cover the internal control service name; replace control.crt and control.key"

}

ensure_secret_files() {
    if [[ ! -f "$SECRETS_DIR/encryption.key" ]]; then
        openssl rand -hex 32 > "$SECRETS_DIR/encryption.key"
    fi
    if [[ ! -f "$SECRETS_DIR/session-signing.pem" ]]; then
        openssl genpkey -algorithm Ed25519 -out "$SECRETS_DIR/session-signing.pem"
    fi
    grep -Eq '^[0-9a-fA-F]{64}$' "$SECRETS_DIR/encryption.key" \
        || fail "encryption.key must contain exactly 32 hex-encoded bytes"
    openssl pkey -in "$SECRETS_DIR/session-signing.pem" -text -noout 2>/dev/null | grep -q ED25519 \
        || fail "session-signing.pem must contain an Ed25519 private key"
}

write_config() {
    cat > "$CONFIG_DIR/control.env" <<EOF
CADESTRO_PUBLIC_LISTEN=0.0.0.0:8081
CADESTRO_AGENT_LISTEN=172.30.0.3:8082
CADESTRO_PUBLIC_BASE_URL=https://${CONTROL_DOMAIN}
CADESTRO_AGENT_URL=https://${AGENT_DOMAIN}
CADESTRO_TERMINAL_URL=wss://${CONTROL_DOMAIN}/terminal
CADESTRO_CORS_ORIGINS=https://${CONTROL_DOMAIN}
CADESTRO_TERMINAL_ORIGINS=${CONTROL_DOMAIN}
CADESTRO_TRUSTED_PROXIES=172.29.0.2
CADESTRO_AGENT_PROXY_SOURCES=172.30.0.2
CADESTRO_LOG_LEVEL=info
CADESTRO_LOG_FORMAT=json
CADESTRO_CERTIFICATE_VALIDITY=8760h
CADESTRO_HEARTBEAT_INTERVAL=30s
CADESTRO_ARTIFACT_PATH=/var/lib/cadestro/artifacts
CADESTRO_DATABASE_PATH=/var/lib/cadestro/state/control.db
CADESTRO_BACKUP_PATH=/var/lib/cadestro/backups
CADESTRO_BACKUP_MAX_LAG=26h
CADESTRO_WEBHOOK_URL=
CADESTRO_CA_CERT_FILE=/run/certs/ca.crt
CADESTRO_CA_KEY_FILE=/run/certs/ca.key
CADESTRO_AGENT_TLS_CERT_FILE=/run/certs/control.crt
CADESTRO_AGENT_TLS_KEY_FILE=/run/certs/control.key
CADESTRO_PUBLIC_TLS_CERT_FILE=/run/certs/control.crt
CADESTRO_PUBLIC_TLS_KEY_FILE=/run/certs/control.key
CADESTRO_ENCRYPTION_KEY_FILE=/run/secrets/encryption.key
CADESTRO_SESSION_SIGNING_KEY_FILE=/run/secrets/session-signing.pem
EOF









    cat > "$CONFIG_DIR/web.env" <<EOF
PUBLIC_CONTROL_URL=https://${CONTROL_DOMAIN}
EOF
}

validate_permissions() {
    local private



    for private in "$CERTS_DIR/ca.key" "$CERTS_DIR/control.key" "$SECRETS_DIR"/* "$CONFIG_DIR"/*.env; do
        [[ "$(stat -c '%a' "$private")" =~ ^[0-6]00$ ]] \
            || fail "$private must not be group/world accessible"
    done
    [[ -w "$DATA_DIR/control" && -w "$DATA_DIR/artifacts" && -w "$DATA_DIR/backups" ]] \
        || fail "state, artifact, and backup paths must be writable"
}

main() {
    require_command openssl
    require_command cmp
    require_command stat
    load_environment
    validate_environment

    mkdir -p "$CERTS_DIR" "$CONFIG_DIR" "$SECRETS_DIR" \
        "$DATA_DIR/control" "$DATA_DIR/traefik" "$DATA_DIR/artifacts" "$DATA_DIR/backups"
    chmod 700 "$CERTS_DIR" "$CONFIG_DIR" "$SECRETS_DIR"
    chmod 700 "$DATA_DIR/control" "$DATA_DIR/artifacts" "$DATA_DIR/backups"
    chmod 600 "$SCRIPT_DIR/.env"
    touch "$DATA_DIR/traefik/acme.json"
    chmod 600 "$DATA_DIR/traefik/acme.json"


    ensure_traefik_acme_config

    ensure_ca
    ensure_certificates
    ensure_secret_files
    write_config

    chmod 600 "$CERTS_DIR"/*.key "$SECRETS_DIR"/* "$CONFIG_DIR/control.env" "$CONFIG_DIR/web.env"
    chmod 644 "$CERTS_DIR"/*.crt
    validate_permissions

    info "Deployment material is ready"
    printf '%s\n' \
        "Start:     cd $SCRIPT_DIR && docker compose up -d --wait" \
        "Bootstrap: cd $SCRIPT_DIR && docker compose exec control cadestro bootstrap-admin"
}
# docref: end generated-material

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
    main "$@"
fi
