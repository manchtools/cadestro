#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

fail() { echo "verify: $*" >&2; exit 1; }

for f in README.md LICENSING.md CONTRIBUTING.md SECURITY.md; do
    [[ -f "$f" ]] || fail "missing required file: $f"
done

templates=(.github/ISSUE_TEMPLATE/*.yml)
[[ ${#templates[@]} -gt 0 && -f "${templates[0]}" ]] || fail "no issue templates found"
for t in "${templates[@]}"; do
    python3 -c "import sys, yaml; yaml.safe_load(open(sys.argv[1]))" "$t" \
        || fail "invalid YAML: $t"
done

for m in contract sdk agent server web; do
    if [[ -d "$m" ]]; then
        [[ -f "$m/LICENSE" ]] || fail "module $m/ exists without $m/LICENSE"
    fi
    grep -q "\`$m/\`" LICENSING.md || fail "LICENSING.md does not name module $m/"
done

echo "verify: OK"
