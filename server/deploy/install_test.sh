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
stub_command tar 'exec /usr/bin/tar "$@"'
stub_command docker 'exit 0'
stub_command openssl '[[ "${OPENSSL_MODE:-}" == reject ]] && exit 1; exit 0'
stub_command base64 'cat >/dev/null'
stub_command sha256sum '[[ "${SHA_MODE:-}" == reject ]] && { printf "%064d  %s\n" 0 "$1"; exit 0; }; exec /usr/bin/sha256sum "$@"'
INSTALLER_PATH="$STUB_ROOT/install.sh"
sed 's/__RELEASE_SIGNING_PUBLIC_KEY__/dGVzdA==/' "$DEPLOY_DIR/install.sh" > "$INSTALLER_PATH"
chmod +x "$INSTALLER_PATH"




for required in python3 timeout; do
    command -v "$required" >/dev/null 2>&1 \
        || { printf '%s is required for the guided tests\n' "$required" >&2; exit 1; }
done






FIXTURE_SOURCE="$FIXTURE_ROOT/deploy"
FIXTURE_TARBALL="$FIXTURE_ROOT/release.tar.gz"
mkdir -p "$FIXTURE_SOURCE"
: > "$FIXTURE_SOURCE/compose.yml"
cat > "$FIXTURE_SOURCE/setup.sh" <<'EOF'

printf 'setup-ran\n' > setup-ran-marker
EOF
chmod +x "$FIXTURE_SOURCE/setup.sh"




tar -czf "$FIXTURE_TARBALL" -C "$FIXTURE_SOURCE" .

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
case "\$(basename "\$target")" in
  source.tar.gz) cp "$FIXTURE_TARBALL" "\$target" ;;
  SHA256SUMS) /usr/bin/sha256sum "$FIXTURE_TARBALL" | awk '{print \$1 "  cadestro-deploy.tar.gz"}' > "\$target" ;;
  SHA256SUMS.sig) : > "\$target" ;;
  *) exit 1 ;;
esac
EOF
chmod +x "$STUB_ROOT/download-bin/curl"
cp "$STUB_ROOT/bin/docker" "$STUB_ROOT/bin/openssl" "$STUB_ROOT/bin/base64" "$STUB_ROOT/download-bin/"
cp "$STUB_ROOT/bin/sha256sum" "$STUB_ROOT/download-bin/"


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
        bash "$INSTALLER_PATH"
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
    grep -Fq 'releases/download/v2026.08.09-rc2/cadestro-deploy.tar.gz' "$CALL_LOG" || {
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
        bash "$INSTALLER_PATH"
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
        bash "$INSTALLER_PATH" </dev/null 2>&1)"; then
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
    grep -Fq 'releases/download/v2026.08.09-rc2/cadestro-deploy.tar.gz' "$CALL_LOG" || {
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
    grep -Fq 'must point at this host' <<<"$output" || {
        printf 'the credential stop does not remind about the DNS records: %s\n' "$output" >&2
        return 1
    }
    return 0
}

test_invalid_signature_refuses_before_unpacking() {
    local directory="$1" output
    if output="$(run_install "$directory" RELEASE_TAG=v2026.08.09-rc2 \
        ACME_CHALLENGE=dns01 ACME_DNS_PROVIDER=hetzner \
        PATH="$STUB_ROOT/download-bin:$PATH" OPENSSL_MODE=reject 2>&1)"; then
        printf 'installer accepted an invalid release signature\n' >&2
        return 1
    fi
    grep -Fq 'release checksum signature is invalid' <<<"$output" || {
        printf 'signature failure was not reported: %s\n' "$output" >&2
        return 1
    }
    assert_install_dir_empty "$directory"
}

test_archive_checksum_refuses_before_unpacking() {
    local directory="$1" output
    if output="$(run_install "$directory" RELEASE_TAG=v2026.08.09-rc2 \
        ACME_CHALLENGE=dns01 ACME_DNS_PROVIDER=hetzner \
        PATH="$STUB_ROOT/download-bin:$PATH" SHA_MODE=reject 2>&1)"; then
        printf 'installer accepted a tampered release archive\n' >&2
        return 1
    fi
    grep -Fq 'release checksum mismatch' <<<"$output" || {
        printf 'checksum failure was not reported: %s\n' "$output" >&2
        return 1
    }
    assert_install_dir_empty "$directory"
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
test_invalid_signature_refuses_before_unpacking "$(new_install_dir)"
printf 'PASS invalid release signature is refused before unpacking\n'
test_archive_checksum_refuses_before_unpacking "$(new_install_dir)"
printf 'PASS tampered release archive is refused before unpacking\n'
