#!/usr/bin/env bash
# The services of deploy/compose.yml as an installation runs them, from the file itself (docs/services.md):
#
#   - Meilisearch: a database filled by the version before (UPGRADE_FROM) is taken over by the pinned version, which
#     has to get healthy with the documents still there. A database belongs to the version that wrote it; without the
#     upgrade the new version refuses to start, and so would the search of every installation after an update.
#   - Gotenberg (profile office): a real spreadsheet becomes a PDF through the client of the preview
#     (internal/files, tag office).
#   - Mailpit: a mail sent over SMTP has to come out of its API with subject, recipient and text (internal/mailpit, tag
#     mailpit). UnderDoc sends no mail yet; once M-22 does, this becomes the test of the real dispatch.
#
# This is the acceptance test for updates of these images. Like scripts/check-image.sh it needs Docker and runs in the
# required check of the CI; locally: sg docker -c scripts/check-services.sh
set -euo pipefail
cd "$(dirname "$0")/.."

# The Meilisearch an installation may still run: the pin before the current one. Move it forward now and then; it has
# to stay at v1.12 or later, the oldest database Meilisearch upgrades.
UPGRADE_FROM="getmeili/meilisearch:v1.56.2@sha256:9600db22be3fcbf3d506176d3a73dd70a65c5719d9aa1d2837355b75afb8f4fb"

fail() { printf '✗ %s\n' "$1" >&2; exit 1; }

docker info >/dev/null 2>&1 || fail "the service tests need Docker (locally: sg docker -c scripts/check-services.sh)"

run="ud-services-$$-$RANDOM"
work="$(mktemp -d)"
key="$(od -An -N24 -tx1 /dev/urandom | tr -d ' \n')"
export MEILI_MASTER_KEY="$key"

# Everything as in the file, except what would meet a running installation or a parallel run: no fixed ports, a network
# of this run's own.
cat >"$work/run.yml" <<EOF
services:
  meilisearch:
    ports: !reset []
  gotenberg:
    ports: !override ["127.0.0.1::3000"]
  mailpit:
    ports: !override ["127.0.0.1::1025", "127.0.0.1::8025"]
networks:
  default:
    name: $run
EOF
printf 'services:\n  meilisearch:\n    image: %s\n' "$UPGRADE_FROM" >"$work/before.yml"

compose() { docker compose -p "$run" -f deploy/compose.yml -f "$work/run.yml" --profile office "$@"; }
cleanup() {
    compose down -v --remove-orphans >/dev/null 2>&1 || true
    rm -rf "$work"
}
trap cleanup EXIT

current="$(sed -n 's/^    image: \(getmeili\/meilisearch:.*\)$/\1/p' deploy/compose.yml)"
[[ -n "$current" ]] || fail "deploy/compose.yml runs no Meilisearch"
[[ "$current" != "$UPGRADE_FROM" ]] || fail "UPGRADE_FROM is the pin itself: nothing would be upgraded, move it to an older version"

# meili <method> <path> [<body>] — the API from inside the container: no port leaves it.
meili() {
    local args=(curl -fsS -X "$1" -H "Authorization: Bearer $key" "http://127.0.0.1:7700$2")
    [[ $# -ge 3 ]] && args+=(-H 'Content-Type: application/json' --data-binary "$3")
    compose exec -T meilisearch "${args[@]}"
}
documents() { meili GET /indexes/check/stats | sed -n 's/.*"numberOfDocuments":\([0-9]*\).*/\1/p'; }
logs() { compose logs --no-color --tail 20 meilisearch >&2 || true; }

echo "▸ Meilisearch: a database of ${UPGRADE_FROM%@*}"
compose -f "$work/before.yml" up -d --wait --wait-timeout 120 meilisearch >/dev/null 2>&1 || { logs; fail "${UPGRADE_FROM%@*} does not get healthy"; }
meili POST /indexes/check/documents '[{"id":"a","title":"Kartenzahlung"},{"id":"b","title":"Warteschlange"},{"id":"c","title":"E-1042"}]' >/dev/null ||
    fail "the documents are not taken"
for _ in $(seq 1 60); do
    [[ "$(documents)" == 3 ]] && break
    sleep 0.5
done
[[ "$(documents)" == 3 ]] || fail "the old version does not index the documents"
compose stop meilisearch >/dev/null 2>&1
compose rm -f meilisearch >/dev/null 2>&1

echo "▸ Meilisearch: ${current%@*} on that database"
compose up -d --wait --wait-timeout 120 meilisearch >/dev/null 2>&1 || { logs; fail "${current%@*} does not get healthy on the database of ${UPGRADE_FROM%@*}"; }
version="$(meili GET /version | sed -n 's/.*"pkgVersion":"\([^"]*\)".*/\1/p')"
tag="${current%@*}"
[[ "v$version" == "${tag##*:}" ]] || fail "the running version is $version, not the pin $current"
[[ "$(documents)" == 3 ]] || fail "after the upgrade the documents are gone: $(documents)"
echo "  ✓ healthy, the three documents are there"

echo "▸ Gotenberg: a spreadsheet becomes a PDF"
compose up -d --wait --wait-timeout 120 gotenberg >/dev/null 2>&1 || { compose logs --no-color --tail 20 gotenberg >&2 || true; fail "Gotenberg does not get healthy"; }
port="$(compose port gotenberg 3000 | sed 's/.*://')"
UNDERDOC_TEST_OFFICE_URL="http://127.0.0.1:$port" go test -count=1 -tags office -run '^TestTheOfficeModuleConverts$' -v ./internal/files/ ||
    fail "the office module does not convert through Gotenberg"

echo "▸ Mailpit: a mail goes in over SMTP and comes out of the API"
compose up -d --wait --wait-timeout 120 mailpit >/dev/null 2>&1 || { compose logs --no-color --tail 20 mailpit >&2 || true; fail "Mailpit does not start"; }
smtp_port="$(compose port mailpit 1025 | sed 's/.*://')"
api_port="$(compose port mailpit 8025 | sed 's/.*://')"
UNDERDOC_TEST_MAILPIT_SMTP="127.0.0.1:$smtp_port" UNDERDOC_TEST_MAILPIT_API="127.0.0.1:$api_port" \
    go test -count=1 -tags mailpit -run '^TestMailpitCatchesAMail$' -v ./internal/mailpit/ ||
    fail "Mailpit does not hand a mail sent over SMTP out of its API"

printf '✓ the services of deploy/compose.yml work\n'
