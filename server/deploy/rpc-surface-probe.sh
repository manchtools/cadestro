#!/usr/bin/env sh

























set -eu

BASE_URL="${1:?usage: rpc-surface-probe.sh <base-url> <expected-file> [removed-file]}"
EXPECTED_FILE="${2:?expected-file required}"
REMOVED_FILE="${3:-}"

red()   { printf '\033[0;31m%s\033[0m\n' "$*"; }
green() { printf '\033[0;32m%s\033[0m\n' "$*"; }
info()  { printf '\033[0;36m[rpc-probe]\033[0m %s\n' "$*"; }

[ -r "$EXPECTED_FILE" ] || { red "FAIL: cannot read expected file $EXPECTED_FILE"; exit 1; }




EXPECTED_COUNT="$(grep -c . "$EXPECTED_FILE" 2>/dev/null || true)"
[ -n "$EXPECTED_COUNT" ] || EXPECTED_COUNT=0
if [ "$EXPECTED_COUNT" -lt 1 ]; then
  red "FAIL: expected procedure set is EMPTY — the probe would pass vacuously"
  exit 1
fi
info "probing $EXPECTED_COUNT declared procedures against $BASE_URL"









probe_status() {
  _proc="$1"
  _body="$(curl -sk -o - -w '\n%{http_code}' \
    -X POST "${BASE_URL}${_proc}" \
    -H 'Content-Type: application/json' \
    --data '{}' \
    --max-time 10 2>/dev/null || true)"
  _status="$(printf '%s' "$_body" | tail -n1)"
  _code="$(printf '%s' "$_body" | sed '$d' \
    | grep -o '"code"[[:space:]]*:[[:space:]]*"[^"]*"' | head -n1 \
    | sed 's/.*"\([^"]*\)"$/\1/' || true)"

  if [ "$_status" = "404" ] || [ "$_code" = "unimplemented" ]; then
    echo unserved
  elif [ -z "$_status" ] || [ "$_status" = "000" ]; then
    echo unreachable
  else
    echo served
  fi
}

MISSING=''
UNREACHABLE=''
while IFS= read -r proc; do
  [ -n "$proc" ] || continue
  case "$(probe_status "$proc")" in
    unserved)    MISSING="${MISSING}${proc}
" ;;
    unreachable) UNREACHABLE="${UNREACHABLE}${proc}
" ;;
  esac
done < "$EXPECTED_FILE"

if [ -n "$UNREACHABLE" ]; then
  red "FAIL: procedures could not be reached at all — the server is not answering:"
  printf '%s' "$UNREACHABLE" | sed 's/^/  /'
  exit 1
fi

if [ -n "$MISSING" ]; then
  red "FAIL: declared procedures are NOT served by the running binary:"
  printf '%s' "$MISSING" | sed 's/^/  /'
  red "the contract promises these; the deployed process does not answer them"
  exit 1
fi
green "all $EXPECTED_COUNT declared procedures are served"




if [ -n "$REMOVED_FILE" ] && [ -s "$REMOVED_FILE" ]; then
  REMOVED_COUNT="$(grep -c . "$REMOVED_FILE" 2>/dev/null || true)"
  [ -n "$REMOVED_COUNT" ] || REMOVED_COUNT=0
  info "checking $REMOVED_COUNT removed procedure(s) are NOT served"
  SURPLUS=''
  while IFS= read -r proc; do
    [ -n "$proc" ] || continue
    if [ "$(probe_status "$proc")" = "served" ]; then
      SURPLUS="${SURPLUS}${proc}
"
    fi
  done < "$REMOVED_FILE"

  if [ -n "$SURPLUS" ]; then
    red "FAIL: removed procedures are STILL SERVED by the running binary:"
    printf '%s' "$SURPLUS" | sed 's/^/  /'
    red "deleting an RPC from the contract does nothing while the binary answers on its path"
    exit 1
  fi
  green "all $REMOVED_COUNT removed procedures are correctly unserved"
fi
