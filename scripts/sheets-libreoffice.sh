#!/usr/bin/env bash
# The spreadsheet fixtures of internal/sheet (ADR-31), made by LibreOffice headless in a container: a workbook with
# number formats, dates, formulas with their results, an error, merged cells, a hidden row, column and sheet, names and a
# database range, saved as .xlsx, .ods and .csv, and what LibreOffice displays in each cell, in English and German, as
# the oracle of the number formats. Not part of the CI (LibreOffice is large); run by hand when LibreOffice moves:
#
#   sg docker -c scripts/sheets-libreoffice.sh
#
# The image is the one of T-1 (scripts/t1/Dockerfile.libreoffice), pinned there.
set -euo pipefail
cd "$(dirname "$0")/.."

fail() { printf '✗ %s\n' "$1" >&2; exit 1; }
docker info >/dev/null 2>&1 || fail "this needs Docker (sg docker -c scripts/sheets-libreoffice.sh)"

image="underdoc-sheets-lo:$$-$RANDOM"
out="$(mktemp -d)"
chmod 777 "$out"
cleanup() {
    docker rmi -f "$image" >/dev/null 2>&1 || true
    rm -rf "$out"
}
trap cleanup EXIT

docker build --quiet -f scripts/t1/Dockerfile.libreoffice -t "$image" scripts/t1 >/dev/null || fail "the LibreOffice image does not build"
run() {
    docker run --rm --network none --entrypoint python3 -u "$(id -u):$(id -g)" -e HOME=/tmp -e "LC_ALL=$1" \
        -v "$PWD/scripts/sheets:/work:ro" -v "$out:/out" "$image" /work/make.py "${@:2}"
}
run en_US.UTF-8 make /out
run en_US.UTF-8 show /out en
run de_DE.UTF-8 show /out de
dest=internal/sheet/testdata/libreoffice
mkdir -p "$dest"
cp "$out"/budget.xlsx "$out"/budget.ods "$out"/budget.csv "$out"/display-en.json "$out"/display-de.json "$dest/"
docker run --rm --entrypoint soffice "$image" --version 2>/dev/null | head -1 >"$dest/VERSION"
printf '✓ fixtures in %s\n' "$dest"
