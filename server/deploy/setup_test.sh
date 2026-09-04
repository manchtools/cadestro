#!/usr/bin/env bash

set -euo pipefail

DEPLOY_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEST_ROOT="$(mktemp -d)"
FIXTURE="$TEST_ROOT/valid"
trap 'rm -rf "$TEST_ROOT"' EXIT

mkdir "$FIXTURE"
cp -R "$DEPLOY_DIR/." "$FIXTURE/"
cat > "$FIXTURE/.env" <<'EOF'
CONTROL_DOMAIN=manage.example.test
AGENT_DOMAIN=agents.example.test
ACME_EMAIL=admin@example.test
IMAGE_TAG=latest
OIDC_ISSUER_URL=https://id.example.test
OIDC_CLIENT_ID=cadestro
EOF

bash "$FIXTURE/setup.sh" >/dev/null
bash "$FIXTURE/setup.sh" >/dev/null
docker compose --project-directory "$FIXTURE" --env-file "$FIXTURE/.env" -f "$FIXTURE/compose.yml" config --quiet

[[ "$(stat -c '%a' "$FIXTURE/certs/ca.key")" == 600 ]]
grep -Fxq 'CADESTRO_AGENT_LISTEN=0.0.0.0:8082' "$FIXTURE/config/control.env"
grep -Fxq 'CADESTRO_OIDC_ISSUER_URL=https://id.example.test' "$FIXTURE/config/control.env"
grep -Fxq 'CADESTRO_OIDC_CLIENT_ID=cadestro' "$FIXTURE/config/control.env"
grep -Fq 'PathPrefix(`/cadestro.v1.ControlService`)' "$FIXTURE/traefik/dynamic/routes.yml"
grep -Fq 'address: control:8082' "$FIXTURE/traefik/dynamic/routes.yml"

if rg -i 'terminal|osquery|inventory|backup|artifact|proxyprotocol|scim' "$FIXTURE/config" "$FIXTURE/compose.yml" "$FIXTURE/traefik"; then
    exit 1
fi

expect_setup_failure() {
    local fixture="$1" message="$2" output
    output="$fixture/setup-error.log"
    if bash "$fixture/setup.sh" >"$output" 2>&1; then
        printf 'setup unexpectedly accepted %s\n' "$fixture" >&2
        return 1
    fi
    if ! grep -Fq "$message" "$output"; then
        cat "$output" >&2
        return 1
    fi
}

failures=0

REPLACED_CA="$TEST_ROOT/replaced-ca"
cp -R "$FIXTURE" "$REPLACED_CA"
openssl genpkey -algorithm Ed25519 -out "$REPLACED_CA/certs/ca.key"
openssl req -new -x509 -key "$REPLACED_CA/certs/ca.key" -days 3650 -subj "/CN=Replacement CA/O=Cadestro" -addext "basicConstraints=critical,CA:TRUE" -addext "keyUsage=critical,keyCertSign,cRLSign" -out "$REPLACED_CA/certs/ca.crt"
expect_setup_failure "$REPLACED_CA" "control certificate is invalid for the current CA, purpose, hostname, or time" || failures=1

CLIENT_ONLY="$TEST_ROOT/client-only"
cp -R "$FIXTURE" "$CLIENT_ONLY"
openssl req -new -key "$CLIENT_ONLY/certs/control.key" -subj "/CN=agents.example.test/O=Cadestro" -out "$CLIENT_ONLY/certs/control.csr"
openssl x509 -req -in "$CLIENT_ONLY/certs/control.csr" -CA "$CLIENT_ONLY/certs/ca.crt" -CAkey "$CLIENT_ONLY/certs/ca.key" -set_serial 2 -days 825 -extfile <(printf 'subjectAltName=DNS:agents.example.test\nextendedKeyUsage=clientAuth\nkeyUsage=digitalSignature\n') -out "$CLIENT_ONLY/certs/control.crt"
rm -f "$CLIENT_ONLY/certs/control.csr"
expect_setup_failure "$CLIENT_ONLY" "control certificate is invalid for the current CA, purpose, hostname, or time" || failures=1

EXPIRED="$TEST_ROOT/expired"
cp -R "$FIXTURE" "$EXPIRED"
openssl req -new -key "$EXPIRED/certs/control.key" -subj "/CN=agents.example.test/O=Cadestro" -out "$EXPIRED/certs/control.csr"
mkdir "$EXPIRED/certs/newcerts"
touch "$EXPIRED/certs/index.txt"
printf '1000\n' > "$EXPIRED/certs/serial"
cat > "$EXPIRED/certs/ca.conf" <<EOF
[ca]
default_ca=issuer
[issuer]
database=$EXPIRED/certs/index.txt
new_certs_dir=$EXPIRED/certs/newcerts
certificate=$EXPIRED/certs/ca.crt
private_key=$EXPIRED/certs/ca.key
serial=$EXPIRED/certs/serial
default_md=default
policy=policy
x509_extensions=server
[policy]
commonName=supplied
organizationName=optional
[server]
subjectAltName=DNS:agents.example.test
extendedKeyUsage=serverAuth
keyUsage=digitalSignature
EOF
openssl ca -batch -notext -config "$EXPIRED/certs/ca.conf" -in "$EXPIRED/certs/control.csr" -startdate 20200101000000Z -enddate 20200102000000Z -out "$EXPIRED/certs/control.crt"
rm -f "$EXPIRED/certs/control.csr"
if openssl x509 -in "$EXPIRED/certs/control.crt" -checkend 0 -noout >/dev/null; then
    printf 'expired fixture is current\n' >&2
    exit 1
fi
expect_setup_failure "$EXPIRED" "control certificate is invalid for the current CA, purpose, hostname, or time" || failures=1

exit "$failures"
