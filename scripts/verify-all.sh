#!/usr/bin/env bash
# The whole-repository gate: every module's own gate, the deployment shell
# tests, and the root structural check — in one run, one at a time.
#
# scripts/verify.sh stays the FAST structural gate (scaffolding, licensing map,
# predecessor-name guard) and is what you run while working. This script is
# what the consolidation gate and a contributor run before handing work over.
#
# STRICTLY SEQUENTIAL, on purpose. Several of these gates are heavy in the same
# resource at the same time — the Go build cache, the module download cache,
# container builds, and a browser download for the web suite. Running them
# concurrently on one machine does not halve the wall clock; it exhausts the
# cache directory and the disk, and the failure it produces is a misleading
# build error rather than an honest test failure. So there is no `&`, no
# `wait`, and no job control anywhere below: gate N+1 starts only after gate N
# has exited and its status has been recorded.
#
# FAIL FAST. The first non-zero gate stops the run. A gate that fails usually
# invalidates what the later ones would be certifying — a contract that does
# not build makes an agent test failure uninformative — so continuing produces
# noise, not more information. The summary still prints, showing which gates
# ran, which failed, and which were never reached, because "not run" and
# "passed" must never look the same.
set -uo pipefail

cd "$(dirname "$0")/.."
ROOT=$PWD

# Every Go module is verified standalone. The repository root carries a go.work
# stitching the four modules together for editing; a gate that ran inside the
# workspace would certify that the modules build TOGETHER, which is not the
# claim — each module must build on its own, or it has an undeclared
# dependency.
export GOWORK=off

names=()
codes=()
failed=""

# run <name> <dir> <command...>
#
# Executes one gate to completion in its own directory and records its real
# exit status. The status is observed, never inferred: a gate whose output
# looks fine but exited non-zero is a failure, and a gate that could not run at
# all is a failure too — not a silent skip.
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

# --- the module gates, in dependency order ---------------------------------
#
# Dependency order, so the first failure is the one closest to the cause: the
# contract is the leaf everything speaks, the SDK sits on it, and the agent and
# the server sit on both. A contract break surfaces as a contract failure
# rather than as four confusing consumer failures.

# The contract owns a canonical gate script: build, vet, staticcheck, tests,
# buf lint and format, generated-code drift, and the TypeScript half.
run "contract" contract ./scripts/verify.sh

# The SDK owns one too.
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
go test -count=1 ./...
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
go test -count=1 -timeout 30m ./...
'

# The web suite runs check BEFORE the tests, in that order: svelte-check
# compiles the paraglide messages and runs `svelte-kit sync`, which the test
# run needs to have happened. Reversing them fails on generated files that do
# not exist yet rather than on anything real.
#
# `npm test` includes the browser project, which drives a real Chromium. On a
# machine that has never run it, install the browser once with
# `npx playwright install --with-deps chromium`; CI does the same before its
# test step.
run "web" web bash -c '
set -euo pipefail
echo "== install"
npm ci
echo "== check"
npm run check
echo "== test"
npm test
'

# The deployment is shell, and so are its tests — `go test ./...` in server/
# does not run them. CI runs them as extra steps of the server Tests job; here
# they are a gate of their own so that a contributor who runs this script gets
# the same coverage CI does, and so a deployment break is reported as a
# deployment failure rather than hidden inside the server gate's output.
run "deploy shell tests" server bash -c '
set -euo pipefail
for t in deploy/setup_test.sh deploy/install_test.sh deploy/backup_test.sh; do
    echo "== $t"
    bash "$t"
done
'

# Last, not first: the structural gate is cheap, but its predecessor-name scan
# reads the whole tree, and running it after the module gates means it sees the
# tree those gates just certified rather than one they might still rewrite
# (generated code, formatted files).
run "root structure" . ./scripts/verify.sh

# The judge compares the working tree with the archived pre-cutover baseline.
# Keep it in the canonical all-gates path so its PASS cannot be mistaken for a
# completed cutover unless the simplification invariants also hold.
run "cutover judge" . python3 tools/cutover_judge.py --repo "$ROOT" --candidate .

# --- summary ---------------------------------------------------------------
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
