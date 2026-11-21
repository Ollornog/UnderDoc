#!/usr/bin/env bash
# Tests the working copy against real Samba (ADR-30, docs/samba.md): builds the companion image (deploy/samba/Dockerfile,
# with smbclient for the tests), starts two companions, one for the tenant main and one for acme, each on the directory
# of its tenant only, and runs the tests with the tag samba against them. Like scripts/check-image.sh it needs Docker and
# runs in the required check of the CI; locally: sg docker -c scripts/check-samba.sh
set -euo pipefail
cd "$(dirname "$0")/.."

fail() { printf '✗ %s\n' "$1" >&2; exit 1; }

docker info >/dev/null 2>&1 || fail "the Samba tests need Docker (locally: sg docker -c scripts/check-samba.sh)"
dock() { docker "$@"; }

run="ud-samba-$$-$RANDOM"
image="underdoc-samba-test:$run"
work="$(mktemp -d)"
chmod 755 "$work"
cleanup() {
    for t in main acme; do dock rm -f -v "$run-$t" >/dev/null 2>&1 || true; done
    dock rmi -f "$image" >/dev/null 2>&1 || true
    # Files the companions made belong to the UIDs of Samba users; a container of the same image takes them away.
    dock run --rm -v "$work:$work" --entrypoint rm debian:trixie-slim@sha256:a72b4433de1cf7d6bae0c4cee74d0547f58601a643b3375f070c51600d63b6b2 -rf "$work/share" >/dev/null 2>&1 || true
    rm -rf "$work"
}
trap cleanup EXIT

dock build --quiet --target test -f deploy/samba/Dockerfile -t "$image" . >/dev/null || fail "the companion image does not build"

mkdir -p "$work/share/main" "$work/share/acme" "$work/sock/main" "$work/sock/acme" "$work/bin"
for t in main acme; do
    dock run -d --name "$run-$t" --label underdoc-samba-test="$run" \
        --cap-drop ALL --cap-add CHOWN --cap-add DAC_OVERRIDE --cap-add FOWNER --cap-add SETUID --cap-add SETGID \
        --cap-add KILL --security-opt no-new-privileges --read-only \
        --tmpfs /var/lib/underdoc-samba:mode=0755 --tmpfs /run/samba --tmpfs /tmp \
        -e UNDERDOC_UID="$(id -u)" -e UNDERDOC_GID="$(id -g)" -e UNDERDOC_SAMBA_ROOT="$work/share/$t" \
        -e UNDERDOC_SAMBA_NAME="ud-$t" \
        -v "$work/share/$t:$work/share/$t" -v "$work/sock/$t:/run/underdoc-samba" "$image" >/dev/null ||
        fail "the companion of $t does not start"
    # The tests talk SMB through smbclient inside the companion, as a client on the same host would.
    printf '#!/usr/bin/env bash\nexec docker exec -i %q smbclient "$@"\n' "$run-$t" >"$work/bin/smbclient-$t"
    chmod 755 "$work/bin/smbclient-$t"
done
for t in main acme; do
    for _ in $(seq 1 60); do
        [[ "$(dock inspect -f '{{.State.Health.Status}}' "$run-$t")" == healthy ]] && break
        sleep 0.5
    done
    [[ "$(dock inspect -f '{{.State.Health.Status}}' "$run-$t")" == healthy ]] ||
        { dock logs "$run-$t" 2>&1 | tail -20 >&2; fail "the companion of $t does not get healthy"; }
done

export UNDERDOC_TEST_SAMBA_DIR="$work/share"
export UNDERDOC_TEST_SAMBA_SOCKETS="main=$work/sock/main/helper.sock,acme=$work/sock/acme/helper.sock"
export UNDERDOC_TEST_SAMBA_CLIENTS="main=$work/bin/smbclient-main,acme=$work/bin/smbclient-acme"
go test -count=1 -tags samba -run 'Samba' ./internal/share/ || {
    for t in main acme; do dock logs "$run-$t" 2>&1 | tail -30 >&2; done
    fail "the tests against Samba"
}
printf '✓ Samba\n'
