#!/usr/bin/env bash
# The JavaScript part of the gate (ADR-21): for each package under web/, install exactly what its lockfile names, run its
# tests, build it into a fresh directory and compare that with the bundle in the repository. A bundle that does not come
# exactly from the sources and the lockfile fails, as does a missing Node.
#
#   scripts/check-js.sh
#
# Repeatable: install and build happen in a temporary copy that is removed; nothing stays in the working tree.
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
    # A copy of the package outside the working tree, without any node_modules of its own: the install leaves nothing
    # behind in the tree (scripts/_residue_check.sh), and nothing installed earlier by hand takes part.
    work="$fresh/work/$dir"
    mkdir -p "$work"
    tar -C "$dir" --exclude=./node_modules -cf - . | tar -C "$work" -xf -
    # --ignore-scripts: no install script of a package runs here; esbuild comes as a package for the platform.
    npm ci --prefix "$work" --ignore-scripts --no-audit --no-fund --loglevel=error || fail "$dir: npm ci"
    if compgen -G "$work/test/*.test.mjs" >/dev/null; then
        node --test --test-reporter=spec "$work"/test/*.test.mjs || fail "$dir: tests"
    fi
    node "$work/build.mjs" --out "$fresh/out/$dir" || fail "$dir: build"
    diff -r "$fresh/out/$dir" "$bundle" >/dev/null ||
        fail "$bundle is not what $dir builds from its lockfile: run 'npm ci --ignore-scripts && npm run build' in $dir and commit the result"
    printf '  the bundle comes exactly from the lockfile\n'
done
