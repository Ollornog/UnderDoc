#!/usr/bin/env bash
# The gate before every push and merge: repository hygiene, the JavaScript part (the web editor built from its lockfile
# and compared with the bundle, its tests), then the Go part (gofmt, tidy, vet, staticcheck, govulncheck, tests with the
# race detector) and the browser test.
#
#   scripts/check.sh            everything
#   scripts/check.sh --fast     without govulncheck and the race detector (short loop while building)
#
# Repeatable: no state stays behind.
set -euo pipefail
cd "$(dirname "$0")/.."

FAST=0
[[ "${1:-}" == "--fast" ]] && FAST=1

step() { printf '\n\033[1m▸ %s\033[0m\n' "$1"; }
fail() { printf '\n\033[31m✗ %s\033[0m\n' "$1" >&2; exit 1; }

PY="$(command -v python3 || true)"
[[ -n "$PY" ]] || fail "python3 is missing (needed for the hygiene base)"
export PYTHONDONTWRITEBYTECODE=1

# Text fast path (kit): a text-only change runs just the hygiene including the text checks.
if grund="$(scripts/_nur_text.sh)"; then
    step "$grund"
    "$PY" tests/test_repo.py || fail "repository hygiene"
    exit 0
fi
step "$grund"

step "Repository hygiene (kit)"
"$PY" tests/test_repo.py || fail "repository hygiene"

if command -v shellcheck >/dev/null; then
    step "shellcheck"
    shellcheck scripts/*.sh || fail "shellcheck"
fi

# Before the Go part: the binary embeds the bundles, and the browser test runs them.
step "JavaScript: the bundles come from the lockfiles"
scripts/build-js.sh || fail "JavaScript"

# The search tests run against real engines in throwaway containers (ADR-24). Without Docker this fails: a skipped
# search test is not a green one. Engines already running can be named instead (scripts/search-engines.sh).
if [[ -z "${UNDERDOC_TEST_MEILISEARCH_URL:-}" ]]; then
    step "Search engines in containers"
    engines="$(scripts/search-engines.sh start)" || fail "search engines (they need Docker, see scripts/search-engines.sh)"
    eval "$engines"
    trap 'scripts/search-engines.sh stop "$UNDERDOC_SEARCH_RUN"' EXIT
    echo "  Meilisearch at $UNDERDOC_TEST_MEILISEARCH_URL"
fi

# The browser test sits behind a tag, so it also has to pass vet and staticcheck explicitly.
export GO_PRUEF_TAGS="browser"
if [[ $FAST -eq 1 ]]; then
    scripts/_go_check.sh --fast
else
    scripts/_go_check.sh
    step "Browser test"
    go test -count=1 -tags browser ./internal/browsertest/ || fail "browser test"
fi

step "Residue check"
rc=0
scripts/_residue_check.sh check || rc=$?
case "$rc" in
    0) : ;;
    2) printf '\033[33m! residue undecidable (working tree was already modified)\033[0m\n' ;;
    *) fail "residue check" ;;
esac

printf '\n\033[32m✓ all green\033[0m\n'
