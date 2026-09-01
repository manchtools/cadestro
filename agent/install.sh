#!/usr/bin/env bash

set -euo pipefail

[[ "${EUID}" == 0 ]] || {
    printf 'install.sh must run as root\n' >&2
    exit 1
}

SOURCE="${1:?usage: sudo ./install.sh /path/to/cadestrod}"
[[ -f "$SOURCE" && -x "$SOURCE" ]] || {
    printf '%s must be an executable regular file\n' "$SOURCE" >&2
    exit 1
}

install -m 0755 "$SOURCE" /usr/local/bin/cadestrod
install -d -m 0700 /var/lib/cadestro
/usr/local/bin/cadestrod install-unit
systemctl enable cadestrod.service
