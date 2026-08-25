#!/usr/bin/env bash
set -uo pipefail

cd "$(dirname "$0")/.."
ROOT=$PWD

export GOWORK=off

names=()
codes=()
failed=""

run() {
    local name=$1 dir=$2
    shift 2

    if [[ -n "$failed" ]]; then
        names+=("$name")
        codes+=("-")
        return
    fi

    printf '\n\033[1m== %s\033[0m  (%s)\n' "$name" "$dir"
    if [[ ! -d "$ROOT/$dir" ]]; then
        printf 'verify-all: %s does not exist\n' "$dir" >&2
        names+=("$name")
        codes+=("missing")
        failed=$name
        return
    fi

    local code=0
    ( cd "$ROOT/$dir" && "$@" ) || code=$?

    names+=("$name")
    codes+=("$code")
    if (( code != 0 )); then
        failed=$name
    fi
}


run "contract" contract ./scripts/verify.sh

run "sdk" sdk ./scripts/verify.sh

run "agent" agent bash -c '
set -euo pipefail
echo "== gofmt"
unformatted=$(gofmt -l .)
[ -z "$unformatted" ] || { echo "gofmt would change:"; echo "$unformatted"; exit 1; }
echo "== go build"
go build ./...
echo "== go vet"
go vet ./...
echo "== go test"
go test -p 1 -count=1 ./...
'

run "server" server bash -c '
set -euo pipefail
echo "== gofmt"
unformatted=$(gofmt -l .)
[ -z "$unformatted" ] || { echo "gofmt would change:"; echo "$unformatted"; exit 1; }
echo "== go build"
go build ./...
echo "== go vet"
go vet ./...
echo "== go test"
go test -p 1 -count=1 -timeout 30m ./...
'

run "web" web bash -c '
set -euo pipefail
echo "== install"
npm ci
echo "== check"
npm run check
echo "== test"
npm test
'

run "deploy shell tests" server bash -c '
set -euo pipefail
for t in deploy/setup_test.sh deploy/install_test.sh deploy/backup_test.sh; do
    echo "== $t"
    bash "$t"
done
'

run "root structure" . ./scripts/verify.sh

run "cutover judge" . python3 tools/cutover_judge.py --repo "$ROOT" --candidate .

printf '\n\033[1m== verify-all summary\033[0m\n'
for i in "${!names[@]}"; do
    case "${codes[$i]}" in
        0)       printf '  \033[32mpass\033[0m     %s\n' "${names[$i]}" ;;
        -)       printf '  skipped  %s  (a previous gate failed)\n' "${names[$i]}" ;;
        missing) printf '  \033[31mMISSING\033[0m  %s\n' "${names[$i]}" ;;
        *)       printf '  \033[31mFAIL\033[0m %-4s %s\n' "(${codes[$i]})" "${names[$i]}" ;;
    esac
done

if [[ -n "$failed" ]]; then
    printf '\nverify-all: FAILED at %s\n' "$failed" >&2
    exit 1
fi

printf '\nverify-all: every gate green (%d gates)\n' "${#names[@]}"
