#!/usr/bin/env bash
set -euo pipefail

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
migration="$script_dir/upgrade-enrollment-tokens.sql"
tmp_dir=$(mktemp -d)
trap 'rm -rf "$tmp_dir"' EXIT

make_legacy_db() {
  local db=$1 extra=${2:-}
  sqlite3 "$db" <<SQL
CREATE TABLE tokens (
  id TEXT PRIMARY KEY, value_hash TEXT NOT NULL UNIQUE, name TEXT NOT NULL,
  max_uses INTEGER NOT NULL DEFAULT 0, expires_at TIMESTAMP,
  created_at TIMESTAMP, created_by TEXT NOT NULL DEFAULT '',
  disabled BOOLEAN NOT NULL DEFAULT FALSE, is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
  one_time BOOLEAN NOT NULL DEFAULT FALSE, current_uses INTEGER NOT NULL DEFAULT 0,
  owner_id TEXT
);
CREATE TABLE devices (
  id TEXT PRIMARY KEY, registered_at TIMESTAMP, registration_token_id TEXT
    REFERENCES tokens(id) ON DELETE SET NULL
);
$extra
SQL
}

assert_refused_without_mutation() {
  local db=$1 expected=$2
  if sqlite3 -bail "$db" < "$migration" >/dev/null 2>&1; then
    echo "migration unexpectedly succeeded for $expected" >&2
    exit 1
  fi
  test "$(sqlite3 "$db" "SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'tokens';")" = tokens
  test "$(sqlite3 "$db" "SELECT count(*) FROM pragma_table_info('tokens') WHERE name = 'current_uses';")" = 1
}

null_expiry="$tmp_dir/null-expiry.db"
make_legacy_db "$null_expiry"
sqlite3 "$null_expiry" \
  "INSERT INTO tokens(id,value_hash,name) VALUES('t1','digest','legacy');"
assert_refused_without_mutation "$null_expiry" "NULL expiry"

missing_provenance="$tmp_dir/missing-provenance.db"
make_legacy_db "$missing_provenance"
sqlite3 "$missing_provenance" \
  "INSERT INTO devices(id,registered_at) VALUES('d1',CURRENT_TIMESTAMP);"
assert_refused_without_mutation "$missing_provenance" "missing token provenance with empty tokens"

orphan_provenance="$tmp_dir/orphan-provenance.db"
make_legacy_db "$orphan_provenance"
sqlite3 "$orphan_provenance" \
  "INSERT INTO devices(id,registered_at,registration_token_id) VALUES('d1',CURRENT_TIMESTAMP,'missing-token');"
assert_refused_without_mutation "$orphan_provenance" "orphan token provenance"

valid="$tmp_dir/valid.db"
make_legacy_db "$valid"
sqlite3 "$valid" <<'SQL'
INSERT INTO tokens(id,value_hash,name,max_uses,expires_at,current_uses)
VALUES('t1','digest','legacy',1,datetime('now','+1 day'),1);
INSERT INTO devices(id,registered_at,registration_token_id)
VALUES('d1',CURRENT_TIMESTAMP,'t1');
SQL
sqlite3 -bail "$valid" < "$migration" >/dev/null
test "$(sqlite3 "$valid" "SELECT value_hash || ':' || max_uses FROM tokens WHERE id = 't1';")" = digest:1
test "$(sqlite3 "$valid" "SELECT registration_token_id FROM devices WHERE id = 'd1';")" = t1
if sqlite3 "$valid" "UPDATE devices SET registration_token_id = NULL WHERE id = 'd1';" >/dev/null 2>&1; then
  echo "migration lost token provenance update guard" >&2
  exit 1
fi
sqlite3 "$valid" "UPDATE devices SET enrollment_identity_public_key = zeroblob(32) WHERE id = 'd1';"
if sqlite3 "$valid" "UPDATE devices SET enrollment_identity_public_key = randomblob(32) WHERE id = 'd1';" >/dev/null 2>&1; then
  echo "migration lost enrollment identity update guard" >&2
  exit 1
fi
if sqlite3 "$valid" "DELETE FROM tokens WHERE id = 't1';" >/dev/null 2>&1; then
  echo "migration lost token provenance delete guard" >&2
  exit 1
fi

one_time="$tmp_dir/one-time.db"
make_legacy_db "$one_time"
sqlite3 "$one_time" <<'SQL'
INSERT INTO tokens(id,value_hash,name,max_uses,expires_at,one_time,current_uses)
VALUES('t1','digest','legacy',0,datetime('now','+1 day'),1,0);
SQL
sqlite3 -bail "$one_time" < "$migration" >/dev/null
test "$(sqlite3 "$one_time" "SELECT max_uses FROM tokens WHERE id = 't1';")" = 1

counter_mismatch="$tmp_dir/counter-mismatch.db"
make_legacy_db "$counter_mismatch"
sqlite3 "$counter_mismatch" <<'SQL'
INSERT INTO tokens(id,value_hash,name,max_uses,expires_at,current_uses)
VALUES('t1','digest','legacy',2,datetime('now','+1 day'),1);
SQL
assert_refused_without_mutation "$counter_mismatch" "counter discrepancy"

owner_token="$tmp_dir/owner-token.db"
make_legacy_db "$owner_token"
sqlite3 "$owner_token" <<'SQL'
INSERT INTO tokens(id,value_hash,name,max_uses,expires_at,current_uses,owner_id)
VALUES('t1','digest','legacy',0,datetime('now','+1 day'),0,'user-1');
SQL
assert_refused_without_mutation "$owner_token" "owner provenance"

echo "upgrade-enrollment-tokens fixtures: PASS"
