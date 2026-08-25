#!/usr/bin/env bash

set -euo pipefail

DEPLOY_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"



CONTROL_ENV_VARIABLES=(
    CADESTRO_PUBLIC_LISTEN
    CADESTRO_AGENT_LISTEN
    CADESTRO_PUBLIC_BASE_URL
    CADESTRO_AGENT_URL
    CADESTRO_TERMINAL_URL
    CADESTRO_CORS_ORIGINS
    CADESTRO_TERMINAL_ORIGINS
    CADESTRO_TRUSTED_PROXIES
    CADESTRO_AGENT_PROXY_SOURCES
    CADESTRO_LOG_LEVEL
    CADESTRO_LOG_FORMAT
    CADESTRO_CERTIFICATE_VALIDITY
    CADESTRO_HEARTBEAT_INTERVAL
    CADESTRO_ARTIFACT_PATH
    CADESTRO_DATABASE_PATH
    CADESTRO_BACKUP_PATH
    CADESTRO_BACKUP_MAX_LAG
    CADESTRO_WEBHOOK_URL
    CADESTRO_CA_CERT_FILE
    CADESTRO_CA_KEY_FILE
    CADESTRO_AGENT_TLS_CERT_FILE
    CADESTRO_AGENT_TLS_KEY_FILE
    CADESTRO_PUBLIC_TLS_CERT_FILE
    CADESTRO_PUBLIC_TLS_KEY_FILE
    CADESTRO_ENCRYPTION_KEY_FILE
    CADESTRO_SESSION_SIGNING_KEY_FILE
)





WEB_ENV_VARIABLES=(
    PUBLIC_CONTROL_URL
)


new_fixture() {
    local directory="$1" control_domain="$2" agent_domain="$3"
    mkdir -p "$directory"
    cat > "$directory/.env" <<EOF
CONTROL_DOMAIN=$control_domain
AGENT_DOMAIN=$agent_domain
ACME_EMAIL=admin@example.test
EOF
}



assert_env_line() {
    local file="$1" line="$2"
    grep -Fxq -- "$line" "$file" || {
        printf 'missing %s in %s\n' "$line" "$file" >&2
        return 1
    }
}




assert_env_variable_set() {
    local file="$1" expected actual
    shift
    expected="$(printf '%s\n' "$@" | LC_ALL=C sort)"
    actual="$(cut -d= -f1 < "$file" | LC_ALL=C sort)"
    [[ "$expected" == "$actual" ]] || {
        printf 'unexpected variables in %s:\n%s\n' "$file" \
            "$(diff <(printf '%s\n' "$expected") <(printf '%s\n' "$actual") || true)" >&2
        return 1
    }
}



env_value() {
    local file="$1" name="$2" matches value
    matches="$(grep -c "^$name=" "$file" || true)"
    [[ "$matches" == 1 ]] || {
        printf '%s is set %s times in %s, want once\n' "$name" "$matches" "$file" >&2
        return 1
    }
    value="$(sed -n "s|^$name=||p" "$file")"
    [[ -n "$value" ]] || {
        printf '%s is empty in %s\n' "$name" "$file" >&2
        return 1
    }
    printf '%s\n' "$value"
}





host_mount_for() {
    local container_path="$1" directory="$2" matches source
    matches="$(grep -cE "^ +- \./[^:]+:${container_path}:" "$DEPLOY_DIR/compose.yml" || true)"
    [[ "$matches" == 1 ]] || {
        printf 'compose.yml bind-mounts %s %s times, want once\n' "$container_path" "$matches" >&2
        return 1
    }
    source="$(sed -nE "s|^ +- \./([^:]+):${container_path}:.*$|\1|p" "$DEPLOY_DIR/compose.yml")"
    printf '%s\n' "$directory/$source"
}




write_dns_credentials() {
    local directory="$1" mode="$2" contents="${3-}"
    mkdir -p "$directory/config"
    printf '%s' "$contents" > "$directory/config/traefik-dns.env"
    chmod "$mode" "$directory/config/traefik-dns.env"
}




challenge_fixture() {
    local directory
    directory="$(mktemp -d "$CHALLENGE_ROOT/XXXXXX")"
    new_fixture "$directory" manage.example.test agents.example.test
    printf '%s\n' "$directory"
}



env_fixture() {
    local directory
    directory="$(mktemp -d "$ENV_ROOT/XXXXXX")"
    new_fixture "$directory" manage.example.test agents.example.test
    printf '%s\n' "$directory"
}




assert_setup_refused() {
    local directory="$1" expected="$2" output artifact
    if output="$(run_setup "$directory" 2>&1)"; then
        printf 'setup.sh accepted an unusable ACME challenge configuration\n' >&2
        return 1
    fi
    grep -Fq -- "$expected" <<<"$output" || {
        printf 'refusal does not name the problem (%s): %s\n' "$expected" "$output" >&2
        return 1
    }
    for artifact in certs/ca.key certs/ca.crt certs/control.key certs/control.crt \
        secrets/encryption.key secrets/session-signing.pem \
        config/control.env config/web.env config/traefik-acme.env; do
        [[ ! -e "$directory/$artifact" ]] || {
            printf 'refused run left %s behind\n' "$directory/$artifact" >&2
            return 1
        }
    done
}




compose_service_environment() {
    local directory="$1" service="${2:-traefik}"
    docker compose -p cadestro-challenge-test -f "$directory/compose.yml" config --format json \
        | python3 -c 'import json, sys
service = json.load(sys.stdin)["services"][sys.argv[1]]["environment"]
print("\n".join(f"{name}={value}" for name, value in service.items()))' "$service"
}





assert_service_healthcheck() {
    local directory="$1" service="$2" expected="$3" command_line
    command_line="$(docker compose -p cadestro-challenge-test -f "$directory/compose.yml" config --format json \
        | python3 -c 'import json, sys
service = json.load(sys.stdin)["services"][sys.argv[1]]
print(" ".join(service.get("healthcheck", {}).get("test", [])))' "$service")"
    [[ "$command_line" == *"$expected"* ]] || {
        printf '%s healthcheck is "%s", want one running %s\n' "$service" "$command_line" "$expected" >&2
        return 1
    }
}

run_setup() {
    local directory="$1"
    (

        cd "$DEPLOY_DIR"
        source setup.sh


        export SCRIPT_DIR="$directory"

        export CERTS_DIR="$directory/certs"

        export CONFIG_DIR="$directory/config"

        export SECRETS_DIR="$directory/secrets"

        export DATA_DIR="$directory/data"
        main
    )
}

test_secure_idempotent_setup() {
    local directory="$1"
    new_fixture "$directory" manage.example.test agents.example.test
    run_setup "$directory" >/dev/null

    [[ "$(stat -c '%a' "$directory/secrets/encryption.key")" == 600 ]]
    [[ "$(stat -c '%a' "$directory/certs/ca.key")" == 600 ]]

    local config="$directory/config/control.env"
    [[ -f "$config" ]]
    [[ "$(stat -c '%a' "$config")" == 600 ]]

    [[ ! -e "$directory/config/control.json" ]]
    assert_env_variable_set "$config" "${CONTROL_ENV_VARIABLES[@]}"
    assert_env_line "$config" 'CADESTRO_PUBLIC_LISTEN=0.0.0.0:8081'
    assert_env_line "$config" 'CADESTRO_AGENT_LISTEN=172.30.0.3:8082'
    assert_env_line "$config" 'CADESTRO_PUBLIC_BASE_URL=https://manage.example.test'
    assert_env_line "$config" 'CADESTRO_AGENT_URL=https://agents.example.test'
    assert_env_line "$config" 'CADESTRO_TERMINAL_URL=wss://manage.example.test/terminal'
    assert_env_line "$config" 'CADESTRO_CORS_ORIGINS=https://manage.example.test'
    assert_env_line "$config" 'CADESTRO_TERMINAL_ORIGINS=manage.example.test'
    assert_env_line "$config" 'CADESTRO_TRUSTED_PROXIES=172.29.0.2'
    assert_env_line "$config" 'CADESTRO_AGENT_PROXY_SOURCES=172.30.0.2'

    assert_env_line "$config" 'CADESTRO_LOG_LEVEL=info'
    assert_env_line "$config" 'CADESTRO_LOG_FORMAT=json'
    assert_env_line "$config" 'CADESTRO_CERTIFICATE_VALIDITY=8760h'
    assert_env_line "$config" 'CADESTRO_HEARTBEAT_INTERVAL=30s'
    assert_env_line "$config" 'CADESTRO_ARTIFACT_PATH=/var/lib/cadestro/artifacts'
    assert_env_line "$config" 'CADESTRO_DATABASE_PATH=/var/lib/cadestro/state/control.db'
    assert_env_line "$config" 'CADESTRO_BACKUP_PATH=/var/lib/cadestro/backups'
    assert_env_line "$config" 'CADESTRO_BACKUP_MAX_LAG=26h'
    assert_env_line "$config" 'CADESTRO_WEBHOOK_URL='
    assert_env_line "$config" 'CADESTRO_CA_CERT_FILE=/run/certs/ca.crt'
    assert_env_line "$config" 'CADESTRO_CA_KEY_FILE=/run/certs/ca.key'
    assert_env_line "$config" 'CADESTRO_AGENT_TLS_CERT_FILE=/run/certs/control.crt'
    assert_env_line "$config" 'CADESTRO_AGENT_TLS_KEY_FILE=/run/certs/control.key'
    assert_env_line "$config" 'CADESTRO_PUBLIC_TLS_CERT_FILE=/run/certs/control.crt'
	assert_env_line "$config" 'CADESTRO_PUBLIC_TLS_KEY_FILE=/run/certs/control.key'
	assert_env_line "$config" 'CADESTRO_ENCRYPTION_KEY_FILE=/run/secrets/encryption.key'
	assert_env_line "$config" 'CADESTRO_SESSION_SIGNING_KEY_FILE=/run/secrets/session-signing.pem'

    if grep -R -iEq 'valkey|asynq|indexer|password_auth|postgres|database_url' "$directory/config"; then
        return 1
    fi
	[[ ! -e "$directory/certs/postgres.crt" ]]
	[[ ! -e "$directory/secrets/postgres.password" ]]



    [[ "$(openssl x509 -in "$directory/certs/control.crt" -checkhost agents.example.test -noout 2>/dev/null)" \
        == "Hostname agents.example.test does match certificate" ]]
    [[ "$(openssl x509 -in "$directory/certs/control.crt" -checkhost control -noout 2>/dev/null)" \
        == "Hostname control does match certificate" ]]

    local before after
    before="$(sha256sum "$directory/certs/ca.key" "$directory/certs/control.key" "$directory/secrets/encryption.key")"
    run_setup "$directory" >/dev/null
    after="$(sha256sum "$directory/certs/ca.key" "$directory/certs/control.key" "$directory/secrets/encryption.key")"
    [[ "$before" == "$after" ]]
}

test_equal_domains_fail() {
    local directory="$1"
    new_fixture "$directory" manage.example.test manage.example.test
    ! run_setup "$directory" >/dev/null 2>&1
}

test_partial_ca_fails() {
    local directory="$1"
    new_fixture "$directory" manage.example.test agents.example.test
    mkdir -p "$directory/certs"
    printf 'not a complete CA\n' > "$directory/certs/ca.crt"
    ! run_setup "$directory" >/dev/null 2>&1
}

test_example_values_fail() {
    local directory="$1"
    new_fixture "$directory" manage.example.com agents.example.com
    ! run_setup "$directory" >/dev/null 2>&1
}

test_backend_name_missing_fails() {
    local directory="$1"
    new_fixture "$directory" manage.example.test agents.example.test
    run_setup "$directory" >/dev/null
    openssl req -new -key "$directory/certs/control.key" \
        -subj '/CN=agents.example.test/O=Cadestro' \
        -out "$directory/certs/control.csr" >/dev/null 2>&1
    openssl x509 -req -in "$directory/certs/control.csr" \
        -CA "$directory/certs/ca.crt" -CAkey "$directory/certs/ca.key" \
        -CAcreateserial -days 825 \
        -extfile <(printf 'subjectAltName=DNS:agents.example.test\nextendedKeyUsage=serverAuth\nkeyUsage=digitalSignature\n') \
        -out "$directory/certs/control.crt" >/dev/null 2>&1
    ! run_setup "$directory" >/dev/null 2>&1
}





test_env_file_values_are_never_executed() {
    local directory
    directory="$(env_fixture)"
    printf "EVIL=\$(touch %s/pwned)\n" "$directory" >> "$directory/.env"




    run_setup "$directory" >/dev/null
    [[ -f "$directory/config/control.env" ]] || {
        printf 'setup.sh did not complete, so the value was never parsed\n' >&2
        return 1
    }
    [[ ! -e "$directory/pwned" ]] || {
        printf 'setup.sh executed a value out of .env\n' >&2
        return 1
    }
}




test_env_file_rejects_a_non_assignment_line() {
    local directory
    directory="$(env_fixture)"
    printf 'this is not an assignment\n' >> "$directory/.env"
    assert_setup_refused "$directory" 'line 4 is not a KEY=VALUE assignment'
}




test_env_file_rejects_a_quoted_value() {
    local directory
    directory="$(env_fixture)"
    printf 'CONTROL_DOMAIN="quoted.example.test"\n' >> "$directory/.env"
    assert_setup_refused "$directory" 'quotes its value'
}




test_static_traefik_config_names_no_challenge() {
    if grep -Eq 'httpChallenge|dnsChallenge' "$DEPLOY_DIR/traefik/traefik.yml"; then
        printf 'traefik.yml still pins an ACME challenge type\n' >&2
        return 1
    fi
    grep -Fq 'storage: /letsencrypt/acme.json' "$DEPLOY_DIR/traefik/traefik.yml"



    grep -Fq 'ping:' "$DEPLOY_DIR/traefik/traefik.yml" || {
        printf 'traefik.yml does not enable ping, so its healthcheck can never succeed\n' >&2
        return 1
    }
}

test_default_challenge_renders_http01() {
    local directory acme credentials
    directory="$(challenge_fixture)"
    run_setup "$directory" >/dev/null

    acme="$directory/config/traefik-acme.env"
    credentials="$directory/config/traefik-dns.env"
    [[ "$(stat -c '%a' "$acme")" == 600 ]]
    assert_env_line "$acme" 'TRAEFIK_CERTIFICATESRESOLVERS_LETSENCRYPT_ACME_HTTPCHALLENGE_ENTRYPOINT=web'


    [[ "$(wc -l < "$acme")" == 1 ]]



    [[ -f "$credentials" && ! -s "$credentials" ]]
    [[ "$(stat -c '%a' "$credentials")" == 600 ]]
}

test_dns01_renders_provider_and_public_resolvers() {
    local directory acme
    directory="$(challenge_fixture)"
    printf 'ACME_CHALLENGE=dns01\nACME_DNS_PROVIDER=hetzner\n' >> "$directory/.env"
    write_dns_credentials "$directory" 600 $'HETZNER_API_KEY=example-token\n'
    run_setup "$directory" >/dev/null

    acme="$directory/config/traefik-acme.env"
    assert_env_line "$acme" 'TRAEFIK_CERTIFICATESRESOLVERS_LETSENCRYPT_ACME_DNSCHALLENGE_PROVIDER=hetzner'



    assert_env_line "$acme" 'TRAEFIK_CERTIFICATESRESOLVERS_LETSENCRYPT_ACME_DNSCHALLENGE_RESOLVERS=1.1.1.1:53,9.9.9.9:53'



    assert_env_line "$acme" 'TRAEFIK_CERTIFICATESRESOLVERS_LETSENCRYPT_ACME_DNSCHALLENGE_PROPAGATION_DELAYBEFORECHECKS=60s'
    [[ "$(wc -l < "$acme")" == 3 ]]
    [[ "$(stat -c '%a' "$acme")" == 600 ]]
    if grep -q HTTPCHALLENGE "$acme"; then
        printf 'dns01 rendered the http01 entrypoint as well\n' >&2
        return 1
    fi


    [[ "$(cat "$directory/config/traefik-dns.env")" == 'HETZNER_API_KEY=example-token' ]]
}

test_dns01_without_provider_fails() {
    local directory
    directory="$(challenge_fixture)"
    printf 'ACME_CHALLENGE=dns01\n' >> "$directory/.env"
    write_dns_credentials "$directory" 600 $'HETZNER_API_KEY=example-token\n'
    assert_setup_refused "$directory" 'ACME_DNS_PROVIDER is required'
}

test_dns01_without_credentials_fails() {
    local directory
    directory="$(challenge_fixture)"
    printf 'ACME_CHALLENGE=dns01\nACME_DNS_PROVIDER=hetzner\n' >> "$directory/.env"



    assert_setup_refused "$directory" 'traefik-dns.env does not exist'
}

test_dns01_with_empty_credentials_fails() {
    local directory
    directory="$(challenge_fixture)"
    printf 'ACME_CHALLENGE=dns01\nACME_DNS_PROVIDER=hetzner\n' >> "$directory/.env"
    write_dns_credentials "$directory" 600 ''
    assert_setup_refused "$directory" 'traefik-dns.env is empty'
}

test_dns01_with_readable_credentials_fails() {
    local directory
    directory="$(challenge_fixture)"
    printf 'ACME_CHALLENGE=dns01\nACME_DNS_PROVIDER=hetzner\n' >> "$directory/.env"
    write_dns_credentials "$directory" 644 $'HETZNER_API_KEY=example-token\n'


    assert_setup_refused "$directory" 'traefik-dns.env must not be group/world accessible'
}

test_unknown_challenge_fails() {
    local directory
    directory="$(challenge_fixture)"
    printf 'ACME_CHALLENGE=bogus\n' >> "$directory/.env"
    assert_setup_refused "$directory" 'ACME_CHALLENGE must be http01 or dns01'
}



test_provider_credentials_never_leave_their_file() {
    local directory output leaked
    directory="$(challenge_fixture)"
    printf 'ACME_CHALLENGE=dns01\nACME_DNS_PROVIDER=hetzner\n' >> "$directory/.env"
    write_dns_credentials "$directory" 600 $'HETZNER_API_KEY=CANARY_SECRET_VALUE_9X7\n'

    output="$(run_setup "$directory" 2>&1)"


    assert_env_line "$directory/config/traefik-acme.env" \
        'TRAEFIK_CERTIFICATESRESOLVERS_LETSENCRYPT_ACME_DNSCHALLENGE_PROVIDER=hetzner'
    if grep -Fq CANARY_SECRET_VALUE_9X7 <<<"$output"; then
        printf 'setup.sh printed the provider credential\n' >&2
        return 1
    fi
    leaked="$(grep -rlF CANARY_SECRET_VALUE_9X7 "$directory" \
        | grep -vFx "$directory/config/traefik-dns.env" || true)"
    [[ -z "$leaked" ]] || {
        printf 'provider credential copied into: %s\n' "$leaked" >&2
        return 1
    }
}




test_web_env_is_rendered_for_the_control_domain() {
    local directory config
    directory="$(challenge_fixture)"
    run_setup "$directory" >/dev/null

    config="$directory/config/web.env"
    [[ -f "$config" ]] || {
        printf 'setup.sh rendered no web configuration; the UI container would start unconfigured\n' >&2
        return 1
    }



    [[ "$(stat -c '%a' "$config")" == 600 ]]
    assert_env_variable_set "$config" "${WEB_ENV_VARIABLES[@]}"





    assert_env_line "$config" 'PUBLIC_CONTROL_URL=https://manage.example.test'
    [[ "$(env_value "$config" PUBLIC_CONTROL_URL)" == "$(env_value "$directory/config/control.env" CADESTRO_PUBLIC_BASE_URL)" ]] || {
        printf 'the UI would call an origin other than the one control publishes its setup URL on\n' >&2
        return 1
    }
}






control_rpc_prefix() {
    local generated="$DEPLOY_DIR/../../contract/gen/go/cadestro/v1/cadestrov1connect/control.connect.go" name
    [[ -f "$generated" ]] || {
        printf 'the generated Connect client is not at %s, so the routed RPC prefix cannot be derived\n' \
            "$generated" >&2
        return 1
    }
    name="$(sed -nE 's|^[[:space:]]*ControlServiceName = "([^"]+)"$|\1|p' "$generated")"
    [[ -n "$name" ]] || {
        printf 'ControlServiceName is not declared in %s\n' "$generated" >&2
        return 1
    }
    printf '/%s\n' "$name"
}




test_traefik_reserves_the_control_paths_and_serves_the_ui() {
    local routes="$DEPLOY_DIR/traefik/dynamic/routes.yml" prefix path
    prefix="$(control_rpc_prefix)"

    grep -Fq "PathPrefix(\`${prefix}\`)" "$routes" || {
        printf 'the control router does not reserve %s; browser RPCs would reach the UI container\n' \
            "$prefix" >&2
        return 1
    }


    for path in '/scim' '/terminal' '/health' '/ready'; do
        grep -Eq "Path(Prefix)?\(\`${path}\`\)" "$routes" || {
            printf 'the control router does not reserve %s\n' "$path" >&2
            return 1
        }
    done
    grep -Fq 'service: web' "$routes" || {
        printf 'no router hands the browser host to the web container\n' >&2
        return 1
    }
    grep -Fq 'url: http://web:3000' "$routes" || {
        printf 'the web service does not name the UI container\n' >&2
        return 1
    }



    local control_priority web_priority
    control_priority="$(sed -nE '/^    control:$/,/^    [a-z]/ s|^ *priority: ([0-9]+)$|\1|p' "$routes")"
    web_priority="$(sed -nE '/^    web:$/,/^    [a-z]/ s|^ *priority: ([0-9]+)$|\1|p' "$routes")"
    [[ -n "$control_priority" && -n "$web_priority" ]] || {
        printf 'both routers must set an explicit priority (control=%s web=%s)\n' \
            "$control_priority" "$web_priority" >&2
        return 1
    }
    (( control_priority > web_priority )) || {
        printf 'the UI catch-all outranks control at %s over %s\n' "$web_priority" "$control_priority" >&2
        return 1
    }
}






test_deploy_pulls_every_declared_service() {
    local declared service pull_line
    mapfile -t declared < <(python3 -c 'import sys, yaml
print("\n".join(sorted(yaml.safe_load(open(sys.argv[1]))["services"])))' "$DEPLOY_DIR/compose.yml")
    [[ ${#declared[@]} -gt 0 ]] || {
        printf 'compose.yml declares no services, so this check proves nothing\n' >&2
        return 1
    }
    pull_line="$(grep -E '^docker compose pull ' "$DEPLOY_DIR/deploy.sh")"
    [[ -n "$pull_line" ]] || {
        printf 'deploy.sh no longer pulls anything by name\n' >&2
        return 1
    }
    for service in "${declared[@]}"; do
        grep -qE "(^| )${service}( |$)" <<<"$pull_line" || {
            printf 'deploy.sh does not pull the %s service: %s\n' "$service" "$pull_line" >&2
            return 1
        }
    done
}



test_compose_configuration_valid_in_both_modes() {
    local directory
    if ! docker compose version >/dev/null 2>&1; then
        printf 'SKIP compose configuration: the Docker Compose plugin is unavailable\n'
        return 0
    fi

    directory="$(challenge_fixture)"
    cp "$DEPLOY_DIR/compose.yml" "$directory/compose.yml"
    run_setup "$directory" >/dev/null
    docker compose -p cadestro-challenge-test -f "$directory/compose.yml" config --quiet
    compose_service_environment "$directory" > "$directory/resolved.env"
    assert_env_line "$directory/resolved.env" \
        'TRAEFIK_CERTIFICATESRESOLVERS_LETSENCRYPT_ACME_HTTPCHALLENGE_ENTRYPOINT=web'
    assert_service_healthcheck "$directory" traefik 'traefik healthcheck'


    compose_service_environment "$directory" web > "$directory/resolved-web.env"
    assert_env_line "$directory/resolved-web.env" 'PUBLIC_CONTROL_URL=https://manage.example.test'

    directory="$(challenge_fixture)"
    cp "$DEPLOY_DIR/compose.yml" "$directory/compose.yml"
    printf 'ACME_CHALLENGE=dns01\nACME_DNS_PROVIDER=hetzner\n' >> "$directory/.env"
    write_dns_credentials "$directory" 600 $'HETZNER_API_KEY=example-token\n'
    run_setup "$directory" >/dev/null
    docker compose -p cadestro-challenge-test -f "$directory/compose.yml" config --quiet
    compose_service_environment "$directory" > "$directory/resolved.env"
    assert_env_line "$directory/resolved.env" \
        'TRAEFIK_CERTIFICATESRESOLVERS_LETSENCRYPT_ACME_DNSCHALLENGE_PROVIDER=hetzner'
    assert_env_line "$directory/resolved.env" \
        'TRAEFIK_CERTIFICATESRESOLVERS_LETSENCRYPT_ACME_DNSCHALLENGE_RESOLVERS=1.1.1.1:53,9.9.9.9:53'


    assert_env_line "$directory/resolved.env" 'HETZNER_API_KEY=example-token'
    printf 'PASS compose configuration valid in both challenge modes\n'
}

fixture_one="$(mktemp -d)"
fixture_two="$(mktemp -d)"
fixture_three="$(mktemp -d)"
fixture_four="$(mktemp -d)"
fixture_six="$(mktemp -d)"
CHALLENGE_ROOT="$(mktemp -d)"
ENV_ROOT="$(mktemp -d)"
trap 'rm -rf "$fixture_one" "$fixture_two" "$fixture_three" "$fixture_four" "$fixture_six" "$CHALLENGE_ROOT" "$ENV_ROOT"' EXIT

test_secure_idempotent_setup "$fixture_one"
printf 'PASS secure and idempotent setup\n'
test_equal_domains_fail "$fixture_two"
printf 'PASS equal domains rejected\n'
test_partial_ca_fails "$fixture_three"
printf 'PASS partial CA rejected\n'
test_example_values_fail "$fixture_four"
printf 'PASS example values rejected\n'
test_backend_name_missing_fails "$fixture_six"
printf 'PASS internal backend name required\n'
test_env_file_values_are_never_executed
printf 'PASS .env values are never executed\n'
test_env_file_rejects_a_non_assignment_line
printf 'PASS .env line that is not an assignment rejected\n'
test_env_file_rejects_a_quoted_value
printf 'PASS .env quoted value rejected\n'
test_static_traefik_config_names_no_challenge
printf 'PASS static Traefik configuration pins no challenge type and enables ping\n'
test_default_challenge_renders_http01
printf 'PASS default ACME challenge renders http01\n'
test_dns01_renders_provider_and_public_resolvers
printf 'PASS dns01 renders the provider and public resolvers\n'
test_dns01_without_provider_fails
printf 'PASS dns01 without a provider rejected\n'
test_dns01_without_credentials_fails
printf 'PASS dns01 without a credentials file rejected\n'
test_dns01_with_empty_credentials_fails
printf 'PASS dns01 with empty credentials rejected\n'
test_dns01_with_readable_credentials_fails
printf 'PASS dns01 with group/world readable credentials rejected\n'
test_unknown_challenge_fails
printf 'PASS unknown ACME challenge rejected\n'
test_provider_credentials_never_leave_their_file
printf 'PASS provider credentials stay in their file\n'
test_web_env_is_rendered_for_the_control_domain
printf 'PASS the UI is preconfigured for the browser domain\n'
test_traefik_reserves_the_control_paths_and_serves_the_ui
printf 'PASS Traefik reserves the control paths and serves the UI on the same origin\n'
test_deploy_pulls_every_declared_service
printf 'PASS deploy.sh pulls every service the deployment declares\n'
test_compose_configuration_valid_in_both_modes
