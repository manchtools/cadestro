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
