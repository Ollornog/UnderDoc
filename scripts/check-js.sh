#!/usr/bin/env bash
# The JavaScript part of the gate (ADR-21): for each package under web/, install exactly what its lockfile names, run its
# tests, build it into a fresh directory and compare that with the bundle in the repository. A bundle that does not come
# exactly from the sources and the lockfile fails, as does a missing Node.
#
#   scripts/check-js.sh
#
# Repeatable: node_modules is ignored by Git, the fresh build goes to a temporary directory that is removed.
set -euo pipefail
cd "$(dirname "$0")/.."

fail() { printf '\n\033[31m✗ %s\033[0m\n' "$1" >&2; exit 1; }

command -v node >/dev/null || fail "node is missing: the JavaScript part cannot be checked (ADR-21)"
command -v npm >/dev/null || fail "npm is missing: the JavaScript part cannot be checked (ADR-21)"
node --version

fresh="$(mktemp -d)"
trap 'rm -rf -- "$fresh"' EXIT

# package directory, and the directory of its bundle in the repository
packages=(
    "web/editor internal/web/assets/app/editor"
    "web/editor-trial internal/web/assets/app/editor-trial"
)
for entry in "${packages[@]}"; do
    read -r dir bundle <<<"$entry"
    printf '\n▸ %s\n' "$dir"
    [[ -f "$dir/package-lock.json" ]] || fail "$dir has no lockfile"
    # --ignore-scripts: no install script of a package runs here; esbuild comes as a package for the platform.
    npm ci --prefix "$dir" --ignore-scripts --no-audit --no-fund --loglevel=error || fail "$dir: npm ci"
    if compgen -G "$dir/test/*.test.mjs" >/dev/null; then
        node --test --test-reporter=spec "$dir"/test/*.test.mjs || fail "$dir: tests"
    fi
    node "$dir/build.mjs" --out "$fresh/$dir" || fail "$dir: build"
    diff -r "$fresh/$dir" "$bundle" >/dev/null ||
        fail "$bundle is not what $dir builds from its lockfile: run 'npm ci --ignore-scripts && npm run build' in $dir and commit the result"
    printf '  the bundle comes exactly from the lockfile\n'
done
