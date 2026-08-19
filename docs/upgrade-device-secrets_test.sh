#!/usr/bin/env bash
set -euo pipefail

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
migration="$script_dir/upgrade-device-secrets.sql"
tmp_dir=$(mktemp -d)
trap 'rm -rf "$tmp_dir"' EXIT

db="$tmp_dir/legacy.db"
sqlite3 "$db" <<'SQL'
CREATE TABLE devices (id TEXT PRIMARY KEY);
CREATE TABLE lps_passwords (id TEXT PRIMARY KEY, device_id TEXT, action_id TEXT, password TEXT);
CREATE TABLE luks_keys (id TEXT PRIMARY KEY, device_id TEXT, action_id TEXT, passphrase TEXT);
PRAGMA user_version = 1;
SQL
sqlite3 -bail "$db" < "$migration" >/dev/null
test "$(sqlite3 "$db" "SELECT count(*) FROM sqlite_master WHERE type='table' AND name='device_secrets';")" = 1
test "$(sqlite3 "$db" "SELECT count(*) FROM sqlite_master WHERE type='table' AND name='lps_passwords';")" = 1
test "$(sqlite3 "$db" "SELECT count(*) FROM sqlite_master WHERE type='table' AND name='luks_keys';")" = 1

echo "upgrade-device-secrets fixture: PASS"
