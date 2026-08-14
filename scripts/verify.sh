#!/usr/bin/env bash
# Repository gate. Grows with the modules: each seeded Go module adds its
# build/vet/test run here, web adds its check+test run. Until then it verifies
# what the repository actually contains — the scaffolding itself.
set -euo pipefail
cd "$(dirname "$0")/.."

fail() { echo "verify: $*" >&2; exit 1; }

# Required root files.
for f in README.md LICENSING.md CONTRIBUTING.md SECURITY.md; do
    [[ -f "$f" ]] || fail "missing required file: $f"
done

# Every issue template must be parseable YAML (matches-zero guarded).
templates=(.github/ISSUE_TEMPLATE/*.yml)
[[ ${#templates[@]} -gt 0 && -f "${templates[0]}" ]] || fail "no issue templates found"
for t in "${templates[@]}"; do
    python3 -c "import sys, yaml; yaml.safe_load(open(sys.argv[1]))" "$t" \
        || fail "invalid YAML: $t"
done

# The rename to Cadestro is an acceptance check, not a best effort: the
# predecessor product name must not reappear anywhere outside the paths listed
# below. Each entry is a file or directory that is ALLOWED to keep the old
# name, and each has to earn it:
#
#   PROVENANCE.md
#       The seed record. It names the predecessor repositories and the SHAs
#       the modules were squashed from, and it is what resolves the
#       `archived <module>#N` citations left in the source. Rewriting it would
#       destroy the audit trail the squash exists to preserve.
#
#   contract/contract_rpc_surface_test.go
#       Carries `abandonedPackages`, a NEGATIVE assertion naming the proto
#       namespaces this contract must never register again. The old names ARE
#       the assertion; sweeping them out would delete the guard, not satisfy
#       it.
#
#   agent/internal/archtest/ci_coverage_test.go
#       Same shape: the forbidden out-of-tree SDK-resolution strings that CI
#       files must never contain. The predecessor repository name is the thing
#       being forbidden.
#
#   web
#       TEMPORARY, and the only entry here that is not permanent. The web
#       module has not been swept yet. The web slice renames it and MUST
#       delete this entry in the same change.
#
# Shrink-only: an allowlist entry that no longer matches anything is itself a
# failure, so the list cannot outlive the occurrences it excuses — that is what
# forces the temporary `web` entry out once the web slice lands.
# The pattern is assembled from a fragment rather than written out, so this
# file does not match its own check. Spelling it literally would force
# scripts/verify.sh onto the allowlist, which would blind the guard to a real
# occurrence in the one file nothing else checks.
old_name_tail='manage'
old_name_re="power[-_ ]?${old_name_tail}|power${old_name_tail}"
old_name_allow=(
    PROVENANCE.md
    contract/contract_rpc_surface_test.go
    agent/internal/archtest/ci_coverage_test.go
    web
)

mapfile -t old_name_hits < <(
    grep -rIiEl --exclude-dir=.git --exclude-dir=node_modules "$old_name_re" . \
        | sed 's#^\./##' | sort
)
# Matches-zero guard on the scan itself. PROVENANCE.md always matches, so an
# empty result means the scan broke (bad regex, wrong directory, missing grep
# feature) rather than a clean tree — the failure mode where this check passes
# precisely because it could not run.
[[ ${#old_name_hits[@]} -gt 0 ]] \
    || fail "predecessor-name scan matched nothing at all, not even PROVENANCE.md — the scan is broken, not the tree"

declare -A old_name_seen=()
old_name_violations=()
for hit in "${old_name_hits[@]}"; do
    excused=""
    for allowed in "${old_name_allow[@]}"; do
        if [[ "$hit" == "$allowed" || "$hit" == "$allowed"/* ]]; then
            excused="$allowed"
            break
        fi
    done
    if [[ -n "$excused" ]]; then
        old_name_seen["$excused"]=1
    else
        old_name_violations+=("$hit")
    fi
done

if [[ ${#old_name_violations[@]} -gt 0 ]]; then
    echo "verify: the predecessor product name survives outside the allowlist:" >&2
    printf '  %s\n' "${old_name_violations[@]}" >&2
    fail "rename these, or justify a new allowlist entry in scripts/verify.sh"
fi

for allowed in "${old_name_allow[@]}"; do
    [[ -n "${old_name_seen[$allowed]:-}" ]] \
        || fail "allowlist entry '$allowed' no longer carries the predecessor name — delete the entry (the allowlist only shrinks)"
done

# Report the pass explicitly. A silent check is indistinguishable from a check
# that never ran, and this one is the acceptance criterion for the rename.
printf 'verify: predecessor name confined to the allowlist (%d matching paths, %d allowlist entries)\n' \
    "${#old_name_hits[@]}" "${#old_name_allow[@]}"

# Every module directory that exists must carry its own LICENSE, and every
# module named in LICENSING.md must be one of the known set (drift guard).
known="contract sdk agent server web"
for m in $known; do
    if [[ -d "$m" ]]; then
        [[ -f "$m/LICENSE" ]] || fail "module $m/ exists without $m/LICENSE"
    fi
    grep -q "\`$m/\`" LICENSING.md || fail "LICENSING.md does not name module $m/"
done

# Per-module gates append below as modules are seeded.

echo "verify: OK"
