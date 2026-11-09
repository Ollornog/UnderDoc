#!/usr/bin/env bash
# Refreshes the vendored design system from a commit of C22. The compiled pack is a build output there, so it is built here.
#
#   scripts/vendor-c22.sh <commit>
set -euo pipefail
cd "$(dirname "$0")/.."

commit="${1:?usage: scripts/vendor-c22.sh <commit>}"
target="internal/web/assets/c22"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

git clone --quiet https://github.com/Ollornog/C22 "$work/c22"
git -C "$work/c22" checkout --quiet "$commit"
"$work/c22/scripts/fetch-tailwind.sh"
"$work/c22/scripts/build-gallery.sh" >/dev/null

cp "$work/c22/c22/static/css/c22-vega.css" "$target/css/"
cp "$work/c22/c22/static/fonts/inter.woff2" "$target/fonts/"
cp "$work/c22/c22/static/js/c22.js" "$work/c22/c22/static/js/workspace-boot.js" "$work/c22/c22/static/js/app-workspace-drag.js" "$target/js/"
cp "$work/c22/c22/vendor/basecoat/dist/js/all.min.js" "$target/js/basecoat.all.min.js"
cp "$work/c22/LICENSE" "$target/licenses/C22-LICENSE"
cp "$work/c22/c22/static/fonts/OFL.txt" "$target/licenses/Inter-OFL.txt"
git -C "$work/c22" rev-parse HEAD >"$target/VERSION"
echo "C22 $(cat "$target/VERSION") vendored. Run scripts/check.sh: it checks every class against the new stylesheet."
