#!/usr/bin/env bash
# Starts the search engines the tests run against (ADR-24), each in a throwaway container with a name and a port of its
# own, so that parallel runs never meet. The images are pinned in scripts/search/Dockerfile.*.
#
#   eval "$(scripts/search-engines.sh start)"   starts Meilisearch and OpenSearch, waits until both answer, prints the
#                                               variables to export
#   scripts/search-engines.sh stop <run>        removes the containers of a run (the run is in UNDERDOC_SEARCH_RUN)
#
# Without Docker it fails and says so: a skipped search test is not a green one.
set -euo pipefail
cd "$(dirname "$0")/.."

fail() { printf '✗ %s\n' "$1" >&2; exit 1; }

# Docker as this shell may use it: directly, or through the docker group of an older login (sg).
dock() {
    if docker info >/dev/null 2>&1; then
        docker "$@"
    elif command -v sg >/dev/null && sg docker -c 'docker info' >/dev/null 2>&1; then
        sg docker -c "docker $(printf '%q ' "$@")"
    else
        return 127
    fi
}

image() { sed -n 's/^FROM //p' "scripts/search/Dockerfile.$1"; }

# wait_for <name> <url> — until the address answers, at most two minutes.
wait_for() {
    local name="$1" url="$2" deadline=$((SECONDS + 120))
    until curl -fsS -o /dev/null "$url" 2>/dev/null; do
        if ((SECONDS > deadline)); then
            dock logs "$name" 2>&1 | tail -20 >&2 || true
            return 1
        fi
        sleep 0.5
    done
}

port() { dock port "$1" "$2/tcp" | head -1 | sed 's/.*://'; }

start() {
    dock info >/dev/null 2>&1 || fail "the search tests need Docker for Meilisearch and OpenSearch: start Docker, or set UNDERDOC_TEST_MEILISEARCH_URL and UNDERDOC_TEST_OPENSEARCH_URL to running engines"
    local run="ud-search-$$-$RANDOM"
    local meili="$run-meilisearch"
    dock run -d --rm --name "$meili" --label underdoc-search-test="$run" -p 127.0.0.1::7700 \
        -e MEILI_NO_ANALYTICS=true -e MEILI_ENV=development "$(image meilisearch)" >/dev/null ||
        fail "Meilisearch does not start"
    # OpenSearch without its security plugin: it listens on loopback only, for the tests. Half a gigabyte of heap is
    # plenty for the corpus.
    local opensearch="$run-opensearch"
    dock run -d --rm --name "$opensearch" --label underdoc-search-test="$run" -p 127.0.0.1::9200 \
        -e discovery.type=single-node -e DISABLE_SECURITY_PLUGIN=true -e DISABLE_INSTALL_DEMO_CONFIG=true \
        -e OPENSEARCH_JAVA_OPTS="-Xms512m -Xmx512m" "$(image opensearch)" >/dev/null ||
        { stop "$run"; fail "OpenSearch does not start"; }
    local meili_url opensearch_url
    meili_url="http://127.0.0.1:$(port "$meili" 7700)"
    opensearch_url="http://127.0.0.1:$(port "$opensearch" 9200)"
    wait_for "$meili" "$meili_url/health" || { stop "$run"; fail "Meilisearch does not answer"; }
    wait_for "$opensearch" "$opensearch_url/_cluster/health?wait_for_status=yellow&timeout=1s" || { stop "$run"; fail "OpenSearch does not answer"; }
    printf 'export UNDERDOC_SEARCH_RUN=%q\n' "$run"
    printf 'export UNDERDOC_TEST_MEILISEARCH_URL=%q\n' "$meili_url"
    printf 'export UNDERDOC_TEST_OPENSEARCH_URL=%q\n' "$opensearch_url"
}

stop() {
    local run="$1"
    [[ "$run" =~ ^ud-search-[0-9]+-[0-9]+$ ]] || fail "not a run of this script: $run"
    local ids
    ids="$(dock ps -aq --filter "label=underdoc-search-test=$run")" || return 0
    [[ -z "$ids" ]] && return 0
    # One id per line; word splitting is wanted here.
    # shellcheck disable=SC2086
    dock rm -f $ids >/dev/null
}

case "${1:-}" in
    start) start ;;
    stop) [[ $# -eq 2 ]] || fail "usage: $0 stop <run>"; stop "$2" ;;
    *) fail "usage: $0 start | stop <run>" ;;
esac
