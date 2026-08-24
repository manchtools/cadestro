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
# Every remaining entry is permanent: each one needs the old name to say what it
# says. The temporary entry this list was born with — `web`, for the module that
# had not been swept yet — is gone, deleted by the slice that swept it.
#
# Shrink-only: an allowlist entry that no longer matches anything is itself a
# failure, so the list cannot outlive the occurrences it excuses. That is what
# forced the temporary entry out, and what stops a permanent one from quietly
# becoming a licence to reintroduce the name somewhere else in its directory.
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

# The predecessor INITIALS are retired too, not just the spelled-out name. They
# were the prefix on the device surface (the runtime directory under /run, the
# per-action sudoers and sshd groups and their drop-in files), on the web design
# tokens, on the local container tags, and on a few hundred test fixtures. All
# of it is renamed, so the spelling must never come back.
#
# The initials ALONE are far too generic to forbid: `rpm-md`, `rpm-build` and
# /etc/pki/rpm-gpg all contain them and all are legitimate. So the check is
# ANCHORED on a word boundary immediately before the initials — which is exactly
# the shape a reintroduced identifier has, and exactly what those rpm spellings
# do not have, because an alphanumeric sits in front of the match there. That
# anchoring is the whole reason this can be a hard failure instead of a grep a
# human has to triage.
#
# There is deliberately NO allowlist. Nothing in the tree may carry this
# spelling — which costs this check the trick the scan above relies on, where
# PROVENANCE.md always matching proves the scan ran. A zero result here is
# ambiguous between "clean tree" and "the grep never worked". So the machinery
# is proved against a POSITIVE CONTROL first: a file that DOES carry the
# forbidden spelling has to be reported before the tree is trusted to be clean.
#
# The scan reads the TRACKED files, not the directory tree. Build output is the
# reason: a stale `web/.svelte-kit/` carries whatever token names the last build
# compiled in, so a recursive scan reports a violation that no source file has
# and that no rename can fix. `git ls-files` is the self-discovering answer —
# "the files this repository contains" — and it needs no hand-maintained list of
# generated directories that would rot the moment a tool changes its output
# path. The list being empty is itself a failure: that means the scan is not
# looking at a repository.
#
# As above, the pattern is assembled from a fragment rather than written out, so
# neither the regex nor the control string makes this file match its own check.
initials_head='p'
initials_re="\\b${initials_head}m-"

initials_control=$(mktemp) || fail "could not create the predecessor-initials positive control"
trap 'rm -f "$initials_control"' EXIT
printf '/run/%sm-agent and group %sm-sudo-01ARZ\n' "$initials_head" "$initials_head" \
    > "$initials_control"
grep -IiE -- "$initials_re" "$initials_control" >/dev/null \
    || fail "predecessor-initials positive control did not match — the scan is broken, not the tree"

mapfile -t initials_tracked < <(git ls-files)
[[ ${#initials_tracked[@]} -gt 0 ]] \
    || fail "predecessor-initials scan found no tracked files — the scan is broken, not the tree"

mapfile -t initials_hits < <(
    printf '%s\0' "${initials_tracked[@]}" \
        | xargs -0 grep -IiEl -- "$initials_re" \
        | sort
)
if [[ ${#initials_hits[@]} -gt 0 ]]; then
    echo "verify: the predecessor initials survive as an identifier prefix:" >&2
    printf '  %s\n' "${initials_hits[@]}" >&2
    fail "rename these to the cadestro- spelling"
fi

printf 'verify: predecessor initials absent as an identifier prefix (positive control matched, tree clean)\n'

# The initials also showed up with no hyphen at all: a Go import alias built
# directly from them (pmv1, pmexec, pmcrypto — real, on ~150 files, before
# they were swept to the package's own name or to a name that says what it
# is instead of who used to own it). The check above cannot see this shape;
# there is no hyphen to anchor on.
#
# The initials alone are too generic to forbid as a bare identifier — "pm" is
# a legitimate two-letter local variable (a package-manager handle is one),
# and the initials open dozens of unrelated test-fixture strings elsewhere in
# the tree (usernames, group names) that this check has no mandate to touch.
# What a reintroduced alias always looks like, and what nothing else does, is
# the shape a Go import spec writes: the alias immediately followed by
# whitespace and the opening quote of the import path. Anchoring there
# instead of on the bare initials is what keeps this a hard failure instead
# of a grep a human has to triage.
#
# Scoped to *.go — that shape cannot occur in any other file type. As above,
# there is no allowlist: nothing in the tree has a reason today to alias an
# import to a name starting with these initials, so a hit here is always a
# violation, and the positive control proves the scan before a zero result is
# trusted. The pattern is assembled from the same fragment as above, so this
# file matches neither check.
alias_re="\\b${initials_head}m[A-Za-z0-9_]*[[:space:]]+\""

alias_control=$(mktemp) || fail "could not create the import-alias positive control"
trap 'rm -f "$initials_control" "$alias_control"' EXIT
printf 'import (\n\t%smv1 "example.com/x"\n)\n' "$initials_head" > "$alias_control"
grep -IiE -- "$alias_re" "$alias_control" >/dev/null \
    || fail "import-alias positive control did not match — the scan is broken, not the tree"

mapfile -t alias_go_files < <(git ls-files -- '*.go')
[[ ${#alias_go_files[@]} -gt 0 ]] \
    || fail "import-alias scan found no tracked Go files — the scan is broken, not the tree"

mapfile -t alias_hits < <(
    printf '%s\0' "${alias_go_files[@]}" \
        | xargs -0 grep -IiEl -- "$alias_re" \
        | sort
)
if [[ ${#alias_hits[@]} -gt 0 ]]; then
    echo "verify: the predecessor initials survive as a Go import alias:" >&2
    printf '  %s\n' "${alias_hits[@]}" >&2
    fail "rename these to the package's real name, or a name that says what it is"
fi

printf 'verify: predecessor initials absent as a Go import alias (positive control matched, tree clean)\n'

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
