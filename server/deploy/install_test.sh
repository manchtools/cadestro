#!/usr/bin/env bash

set -euo pipefail

DEPLOY_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"






STUB_ROOT="$(mktemp -d)"
FIXTURE_ROOT="$(mktemp -d)"
export CALL_LOG="$STUB_ROOT/calls"
trap 'rm -rf "$STUB_ROOT" "$FIXTURE_ROOT"' EXIT

stub_command() {
    local name="$1" body="$2"
    mkdir -p "$STUB_ROOT/bin"
    cat > "$STUB_ROOT/bin/$name" <<EOF

set -euo pipefail
printf '$name %s\n' "\$*" >> "\$CALL_LOG"
$body
EOF
    chmod +x "$STUB_ROOT/bin/$name"
}



stub_command curl 'exit 1'
stub_command tar 'exit 0'
stub_command docker 'exit 0'
stub_command openssl 'exit 0'




for required in python3 timeout; do
    command -v "$required" >/dev/null 2>&1 \
        || { printf '%s is required for the guided tests\n' "$required" >&2; exit 1; }
done






FIXTURE_SOURCE="$FIXTURE_ROOT/cadestro-server-fixture"
FIXTURE_TARBALL="$FIXTURE_ROOT/release.tar.gz"
SKEW_TARBALL="$FIXTURE_ROOT/release-skew.tar.gz"
mkdir -p "$FIXTURE_SOURCE/deploy"
: > "$FIXTURE_SOURCE/deploy/compose.yml"
cat > "$FIXTURE_SOURCE/deploy/setup.sh" <<'EOF'

printf 'setup-ran\n' > setup-ran-marker
EOF
chmod +x "$FIXTURE_SOURCE/deploy/setup.sh"




tar -czf "$SKEW_TARBALL" -C "$FIXTURE_ROOT" "$(basename "$FIXTURE_SOURCE")"
cp "$DEPLOY_DIR/install.sh" "$FIXTURE_SOURCE/deploy/install.sh"
tar -czf "$FIXTURE_TARBALL" -C "$FIXTURE_ROOT" "$(basename "$FIXTURE_SOURCE")"

mkdir -p "$STUB_ROOT/download-bin"
cat > "$STUB_ROOT/download-bin/curl" <<EOF

set -euo pipefail
printf 'curl %s\n' "\$*" >> "\$CALL_LOG"
target=""
previous=""
for argument in "\$@"; do
    [[ "\$previous" == -o ]] && target="\$argument"
    previous="\$argument"
done
cp "$FIXTURE_TARBALL" "\$target"
EOF
chmod +x "$STUB_ROOT/download-bin/curl"
cp "$STUB_ROOT/bin/docker" "$STUB_ROOT/bin/openssl" "$STUB_ROOT/download-bin/"


mkdir -p "$STUB_ROOT/skew-bin"
sed "s|$FIXTURE_TARBALL|$SKEW_TARBALL|" "$STUB_ROOT/download-bin/curl" > "$STUB_ROOT/skew-bin/curl"
chmod +x "$STUB_ROOT/skew-bin/curl"
cp "$STUB_ROOT/bin/docker" "$STUB_ROOT/bin/openssl" "$STUB_ROOT/skew-bin/"

new_install_dir() {
    mktemp -d "$FIXTURE_ROOT/XXXXXX"
}





run_install() {
    local directory="$1"
    shift
    : > "$CALL_LOG"
    env -u RELEASE_TAG -u ACME_CHALLENGE -u ACME_DNS_PROVIDER \
        PATH="$STUB_ROOT/bin:$PATH" \
        INSTALL_DIR="$directory" \
        CONTROL_DOMAIN=manage.example.test \
        AGENT_DOMAIN=agents.example.test \
        ACME_EMAIL=admin@example.test \
        GITHUB_REPOSITORY=cadestro.invalid/server \
        "$@" \
        bash "$DEPLOY_DIR/install.sh"
}

assert_no_download() {
    local attempts
    attempts="$(grep -c '^curl ' "$CALL_LOG" || true)"
    [[ "$attempts" == 0 ]] || {
        printf 'install.sh attempted %s download(s):\n%s\n' "$attempts" "$(grep '^curl ' "$CALL_LOG")" >&2
        return 1
    }
}




assert_install_dir_empty() {
    local directory="$1" leftover
    leftover="$(find "$directory" -mindepth 1 -print -quit)"
    [[ -z "$leftover" ]] || {
        printf 'refused run wrote into the install directory: %s\n' "$leftover" >&2
        return 1
    }
}




test_missing_release_tag_refuses_before_downloading() {
    local directory="$1" output
    if output="$(run_install "$directory" 2>&1)"; then
        printf 'install.sh installed something with RELEASE_TAG unset\n' >&2
        return 1
    fi
    assert_no_download
    grep -Fq 'RELEASE_TAG' <<<"$output" || {
        printf 'refusal does not name RELEASE_TAG: %s\n' "$output" >&2
        return 1
    }


    grep -Eq 'RELEASE_TAG=v[0-9]' <<<"$output" || {
        printf 'refusal does not show a release-tag example: %s\n' "$output" >&2
        return 1
    }
    assert_install_dir_empty "$directory"
}




test_invalid_challenge_refuses_before_downloading() {
    local directory="$1" output
    if output="$(run_install "$directory" RELEASE_TAG=v2026.08.09-rc2 ACME_CHALLENGE=bogus 2>&1)"; then
        printf 'install.sh accepted an unusable ACME challenge\n' >&2
        return 1
    fi
    assert_no_download
    grep -Fq 'ACME_CHALLENGE must be http01 or dns01' <<<"$output" || {
        printf 'refusal does not name the challenge problem: %s\n' "$output" >&2
        return 1
    }
    assert_install_dir_empty "$directory"
}




test_complete_environment_reaches_the_release_tag() {
    local directory="$1"
    if run_install "$directory" RELEASE_TAG=v2026.08.09-rc2 >/dev/null 2>&1; then
        printf 'the stubbed download succeeded; the negative cases prove nothing\n' >&2
        return 1
    fi
    grep -Fq 'refs/tags/v2026.08.09-rc2.tar.gz' "$CALL_LOG" || {
        printf 'install.sh did not request the release tag:\n%s\n' "$(cat "$CALL_LOG")" >&2
        return 1
    }
}






run_install_guided() {
    local directory="$1" answers="$2" bin_directory="$3" fstab="${4:-/dev/null}"
    : > "$CALL_LOG"
    printf '%s' "$answers" | timeout 30 env \
        -u RELEASE_TAG -u ACME_CHALLENGE -u ACME_DNS_PROVIDER \
        -u CONTROL_DOMAIN -u AGENT_DOMAIN -u ACME_EMAIL \
        PATH="$bin_directory:$PATH" \
        INSTALL_DIR="$directory" \
        GITHUB_REPOSITORY=cadestro.invalid/server \
        FSTAB_FILE="$fstab" \
        python3 -c 'import os, pty, sys; sys.exit(os.waitstatus_to_exitcode(pty.spawn(sys.argv[1:])))' \
        bash "$DEPLOY_DIR/install.sh"
}



test_missing_domain_without_terminal_still_refuses() {
    local directory="$1" output
    : > "$CALL_LOG"
    if output="$(env -u CONTROL_DOMAIN \
        PATH="$STUB_ROOT/bin:$PATH" \
        INSTALL_DIR="$directory" \
        AGENT_DOMAIN=agents.example.test \
        ACME_EMAIL=admin@example.test \
        RELEASE_TAG=v2026.08.09-rc2 \
        GITHUB_REPOSITORY=cadestro.invalid/server \
        bash "$DEPLOY_DIR/install.sh" </dev/null 2>&1)"; then
        printf 'install.sh proceeded without CONTROL_DOMAIN and without a terminal\n' >&2
        return 1
    fi
    grep -Fq 'set CONTROL_DOMAIN' <<<"$output" || {
        printf 'refusal does not name CONTROL_DOMAIN: %s\n' "$output" >&2
        return 1
    }
    assert_no_download
    assert_install_dir_empty "$directory"
}




test_guided_answers_reach_the_release_tag() {
    local directory="$1" output answers
    answers=$'not_a_domain\nmanage.example.test\nagents.example.test\nadmin@example.test\nv2026.08.09-rc2\n\n\n'
    output="$(run_install_guided "$directory" "$answers" "$STUB_ROOT/bin" 2>&1)" && {
        printf 'the stubbed download succeeded; the guided case proves nothing\n' >&2
        return 1
    }
    grep -Fq 'could not download' <<<"$output" || {
        printf 'guided run did not reach the download step: %s\n' "$output" >&2
        return 1
    }
    grep -Fq 'refs/tags/v2026.08.09-rc2.tar.gz' "$CALL_LOG" || {
        printf 'guided run did not request the answered release tag:\n%s\n' "$(cat "$CALL_LOG")" >&2
        return 1
    }
    grep -Fq 'fully-qualified hostname' <<<"$output" || {
        printf 'invalid hostname answer was not re-asked with a hint: %s\n' "$output" >&2
        return 1
    }
    grep -Fq 'dns01' <<<"$output" || {
        printf 'certificate choice was never offered: %s\n' "$output" >&2
        return 1
    }
}




test_guided_dns01_marks_the_credential_for_self_pasting() {
    local directory="$1" output answers
    answers=$'manage.example.test\nagents.example.test\nadmin@example.test\nv2026.08.09-rc2\ndns01\nhetzner\n\n'
    output="$(run_install_guided "$directory" "$answers" "$STUB_ROOT/download-bin" 2>&1)" && {
        printf 'guided dns01 run finished although the credential is missing\n' >&2
        return 1
    }
    for expected in \
        'CONTROL_DOMAIN=manage.example.test' \
        'AGENT_DOMAIN=agents.example.test' \
        'ACME_EMAIL=admin@example.test' \
        'ACME_CHALLENGE=dns01' \
        'ACME_DNS_PROVIDER=hetzner' \
        'IMAGE_TAG=2026.08.09-rc2'; do
        grep -Fxq "$expected" "$directory/.env" || {
            printf 'guided .env is missing %s:\n%s\n' "$expected" "$(cat "$directory/.env")" >&2
            return 1
        }
    done
    [[ -f "$directory/config/traefik-dns.env" && ! -s "$directory/config/traefik-dns.env" ]] || {
        printf 'the empty credentials file was not prepared\n' >&2
        return 1
    }
    [[ "$(stat -c '%a' "$directory/config/traefik-dns.env")" == 600 ]] || {
        printf 'credentials file is not mode 600\n' >&2
        return 1
    }
    grep -Fq 'ACTION REQUIRED' <<<"$output" || {
        printf 'the stop does not mark the credential step: %s\n' "$output" >&2
        return 1
    }
    grep -Fq 'traefik-dns.env' <<<"$output" || {
        printf 'the stop does not name the credentials file: %s\n' "$output" >&2
        return 1
    }
    [[ ! -e "$directory/setup-ran-marker" ]] || {
        printf 'setup.sh ran although the credential is missing\n' >&2
        return 1
    }
    grep -Eq 'docker compose (pull|up)' "$CALL_LOG" && {
        printf 'the stack was touched although the credential is missing:\n%s\n' "$(cat "$CALL_LOG")" >&2
        return 1
    }
    grep -Fq 'differs from the one inside release' <<<"$output" && {
        printf 'a matching release tree must not raise the skew warning: %s\n' "$output" >&2
        return 1
    }
    grep -Fq 'must point at this host' <<<"$output" || {
        printf 'the credential stop does not remind about the DNS records: %s\n' "$output" >&2
        return 1
    }
    return 0
}





test_version_skew_between_script_and_tree_warns() {
    local directory="$1" output
    : > "$CALL_LOG"
    if output="$(run_install "$directory" \
        RELEASE_TAG=v2026.08.09-rc2 ACME_CHALLENGE=dns01 ACME_DNS_PROVIDER=hetzner \
        PATH="$STUB_ROOT/skew-bin:$PATH" 2>&1)"; then
        printf 'skewed dns01 run finished although the credential is missing\n' >&2
        return 1
    fi
    grep -Fq 'differs from the one inside release' <<<"$output" || {
        printf 'the version skew was not named: %s\n' "$output" >&2
        return 1
    }
    grep -Fq 'ACTION REQUIRED' <<<"$output" || {
        printf 'the skew warning must not replace the credential stop: %s\n' "$output" >&2
        return 1
    }
}

test_missing_release_tag_refuses_before_downloading "$(new_install_dir)"
printf 'PASS unset RELEASE_TAG refused before any download\n'
test_invalid_challenge_refuses_before_downloading "$(new_install_dir)"
printf 'PASS unusable ACME challenge refused before any download\n'
test_complete_environment_reaches_the_release_tag "$(new_install_dir)"
printf 'PASS a complete environment fetches the named release tag\n'
test_missing_domain_without_terminal_still_refuses "$(new_install_dir)"
printf 'PASS missing value without a terminal refuses instead of prompting\n'
test_guided_answers_reach_the_release_tag "$(new_install_dir)"
printf 'PASS guided answers drive the fetch and invalid input is re-asked\n'
test_guided_dns01_marks_the_credential_for_self_pasting "$(new_install_dir)"
printf 'PASS guided dns01 prepares the credential file and stops before setup.sh\n'
test_version_skew_between_script_and_tree_warns "$(new_install_dir)"
printf 'PASS a tree whose install.sh differs from the running script is named\n'
