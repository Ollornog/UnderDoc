#!/usr/bin/env bash
# Builds the image and starts it hardened on a read-only folder: it has to get healthy, serve a page and notice a change.
# This is also the acceptance test for updates of the base images.
set -euo pipefail
cd "$(dirname "$0")/.."

fail() { printf '✗ %s\n' "$1" >&2; docker logs "$name" 2>&1 | tail -20 >&2 || true; exit 1; }

image="underdoc-check:$$"
name="underdoc-check-$$"
notes="$(mktemp -d)"
trap 'docker rm -f "$name" >/dev/null 2>&1 || true; docker rmi -f "$image" >/dev/null 2>&1 || true; rm -rf "$notes"' EXIT

chmod 755 "$notes"
printf -- '---\nid: first\n---\n# First version\n' >"$notes/page.md"
chmod 644 "$notes/page.md"

docker build --quiet -t "$image" . >/dev/null
docker run -d --name "$name" --read-only --cap-drop ALL --security-opt no-new-privileges \
    -e UNDERDOC_SCAN_INTERVAL=200ms -p 127.0.0.1::8080 -v "$notes:/data:ro" "$image" >/dev/null

for _ in $(seq 1 60); do
    [[ "$(docker inspect -f '{{.State.Health.Status}}' "$name")" == healthy ]] && break
    sleep 0.5
done
[[ "$(docker inspect -f '{{.State.Health.Status}}' "$name")" == healthy ]] || fail "the container does not get healthy"

port="$(docker port "$name" 8080/tcp | head -1 | sed 's/.*://')"
base="http://127.0.0.1:$port"
url="$base/o/first"
# Without the headers of the sign-in gate nothing answers (M-28), with them the workspace and its data show.
code="$(curl -s -o /dev/null -w '%{http_code}' "$url")"
[ "$code" = 401 ] || fail "without the gate the page answered $code, not 401"
gate=(-H 'Remote-User: image-check')
# The answers go into variables first: with pipefail, grep -q could end curl early and fail the pipe at random.
page="$(curl -fsS "${gate[@]}" "$url")" || fail "the workspace does not answer"
[[ "$page" == *'class="c22-app-workspace"'* ]] || fail "the workspace does not show"
head="$(curl -fsSI "${gate[@]}" "$url")" || fail "the workspace does not answer"
grep -qi '^content-security-policy:' <<<"$head" || fail "the page has no CSP"
doc="$(curl -fsS "${gate[@]}" "$base/api/doc/first")" || fail "the page does not answer"
[[ "$doc" == *'First version'* ]] || fail "the page does not show"

printf -- '---\nid: first\n---\n# Second, longer version\n' >"$notes/page.md"
for _ in $(seq 1 40); do
    if doc="$(curl -fsS "${gate[@]}" "$base/api/doc/first")" && [[ "$doc" == *'Second, longer version'* ]]; then
        echo "✓ image: healthy, read-only, closed without the gate, serves the workspace and pages and notices changes"
        exit 0
    fi
    sleep 0.25
done
fail "the change on disk does not show"
