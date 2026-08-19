#!/usr/bin/env bash
set -euo pipefail

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
migration="$script_dir/upgrade-certificate-lifecycle.sql"
tmp_dir=$(mktemp -d)
trap 'rm -rf "$tmp_dir"' EXIT

make_legacy_db() {
  local db=$1 extra=${2:-}
  sqlite3 "$db" <<SQL
CREATE TABLE devices (
  id TEXT PRIMARY KEY, certificate_pem BLOB, cert_fingerprint TEXT,
  cert_not_after TIMESTAMP
);
CREATE TABLE revoked_certificates (fingerprint TEXT PRIMARY KEY);
PRAGMA user_version = 1;
$extra
SQL
}

assert_refused_without_mutation() {
  local db=$1
  if sqlite3 -bail "$db" < "$migration" >/dev/null 2>&1; then
    echo "migration unexpectedly succeeded" >&2
    exit 1
  fi
  test "$(sqlite3 "$db" "SELECT count(*) FROM pragma_table_info('devices') WHERE name = 'active_cert_serial';")" = 0
  test "$(sqlite3 "$db" "SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'revoked_certificates';")" = 1
}

valid="$tmp_dir/valid.db"
make_legacy_db "$valid"
sqlite3 -bail "$valid" < "$migration" >/dev/null
test "$(sqlite3 "$valid" "SELECT count(*) FROM pragma_table_info('devices') WHERE name IN ('active_cert_serial','pending_certificate_pem','pending_cert_serial');")" = 3
test "$(sqlite3 "$valid" "SELECT count(*) FROM sqlite_master WHERE type = 'trigger' AND name IN ('devices_certificate_lifecycle_pair','devices_certificate_lifecycle_pair_update');")" = 2
test "$(sqlite3 "$valid" "SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'revoked_certificates';")" = 0

if sqlite3 -bail "$valid" < "$migration" >/dev/null 2>&1; then
  echo "repeated migration unexpectedly succeeded" >&2
  exit 1
fi
test "$(sqlite3 "$valid" "SELECT count(*) FROM pragma_table_info('devices') WHERE name = 'active_cert_serial';")" = 1

partial="$tmp_dir/partial.db"
make_legacy_db "$partial" "ALTER TABLE devices DROP COLUMN cert_not_after;"
assert_refused_without_mutation "$partial"

echo "upgrade-certificate-lifecycle fixtures: PASS"
