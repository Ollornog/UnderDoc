#!/usr/bin/env bash
# T-1, the part of LibreOffice: LibreOffice headless opens, changes and saves files on a share of a real Samba companion,
# through the kernel's SMB client, while smbd's audit records every operation. The test (tag t1) checks what the working
# copy makes of it and, with UNDERDOC_T1_FIXTURES set, writes what was recorded as fixtures for the tests of the watcher
# (internal/share/testdata/t1). Not part of the CI: it mounts a share inside a container, which needs CAP_SYS_ADMIN, and
# LibreOffice is large. Run by hand when Samba or LibreOffice move:
#
#   sg docker -c scripts/t1-libreoffice.sh                                       measure and check
#   UNDERDOC_T1_FIXTURES=internal/share/testdata/t1 sg docker -c scripts/t1-libreoffice.sh   and write the fixtures
set -euo pipefail
cd "$(dirname "$0")/.."

fail() { printf '✗ %s\n' "$1" >&2; exit 1; }
docker info >/dev/null 2>&1 || fail "T-1 needs Docker (sg docker -c scripts/t1-libreoffice.sh)"

run="ud-t1-$$-$RANDOM"
samba_image="underdoc-samba-test:$run"
lo_image="underdoc-t1-lo:$run"
work="$(mktemp -d)"
chmod 755 "$work"
cleanup() {
    for t in main acme; do docker rm -f -v "$run-$t" >/dev/null 2>&1 || true; done
    docker network rm "$run" >/dev/null 2>&1 || true
    docker rmi -f "$samba_image" "$lo_image" >/dev/null 2>&1 || true
    docker run --rm -v "$work:$work" --entrypoint rm debian:trixie-slim@sha256:a72b4433de1cf7d6bae0c4cee74d0547f58601a643b3375f070c51600d63b6b2 -rf "$work/share" >/dev/null 2>&1 || true
    rm -rf "$work"
}
trap cleanup EXIT

docker build --quiet --target test -f deploy/samba/Dockerfile -t "$samba_image" . >/dev/null || fail "the companion image does not build"
docker build --quiet -f scripts/t1/Dockerfile.libreoffice -t "$lo_image" scripts/t1 >/dev/null || fail "the LibreOffice image does not build"
docker network create --label underdoc-t1="$run" "$run" >/dev/null

mkdir -p "$work/share/main" "$work/share/acme" "$work/sock/main" "$work/sock/acme" "$work/bin"
for t in main acme; do
    docker run -d --name "$run-$t" --network "$run" --label underdoc-t1="$run" \
        --cap-drop ALL --cap-add CHOWN --cap-add DAC_OVERRIDE --cap-add FOWNER --cap-add SETUID --cap-add SETGID \
        --cap-add KILL --security-opt no-new-privileges --read-only \
        --tmpfs /var/lib/underdoc-samba:mode=0755 --tmpfs /run/samba --tmpfs /tmp \
        -e UNDERDOC_UID="$(id -u)" -e UNDERDOC_GID="$(id -g)" -e UNDERDOC_SAMBA_ROOT="$work/share/$t" \
        -e UNDERDOC_SAMBA_NAME="ud-$t" -e UNDERDOC_SAMBA_AUDIT=all \
        -v "$work/share/$t:$work/share/$t" -v "$work/sock/$t:/run/underdoc-samba" "$samba_image" >/dev/null ||
        fail "the companion of $t does not start"
    printf '#!/usr/bin/env bash\nexec docker exec -i %q smbclient "$@"\n' "$run-$t" >"$work/bin/smbclient-$t"
    chmod 755 "$work/bin/smbclient-$t"
done
for t in main acme; do
    for _ in $(seq 1 60); do
        [[ "$(docker inspect -f '{{.State.Health.Status}}' "$run-$t")" == healthy ]] && break
        sleep 0.5
    done
    [[ "$(docker inspect -f '{{.State.Health.Status}}' "$run-$t")" == healthy ]] || fail "the companion of $t does not get healthy"
done
# LibreOffice as a client: lo <user> <password> [--make] <file>... against the share Company of main. The mount needs
# CAP_SYS_ADMIN inside this one container, and no AppArmor profile that forbids mounting.
cat >"$work/bin/lo" <<WRAPPER
#!/usr/bin/env bash
user="\$1" pass="\$2"; shift 2
exec docker run --rm --network $run --cap-add SYS_ADMIN --cap-add DAC_READ_SEARCH --security-opt apparmor=unconfined \\
    -e SMB_USER="\$user" -e SMB_PASS="\$pass" $lo_image "\$@"
WRAPPER
chmod 755 "$work/bin/lo"

export UNDERDOC_TEST_SAMBA_DIR="$work/share"
export UNDERDOC_TEST_SAMBA_SOCKETS="main=$work/sock/main/helper.sock,acme=$work/sock/acme/helper.sock"
export UNDERDOC_TEST_SAMBA_CLIENTS="main=$work/bin/smbclient-main,acme=$work/bin/smbclient-acme"
export UNDERDOC_T1_LO="$work/bin/lo"
export UNDERDOC_T1_SERVER="//$run-main"
export UNDERDOC_T1_VERSIONS
UNDERDOC_T1_VERSIONS="$(docker run --rm --entrypoint soffice "$lo_image" --version 2>/dev/null | head -1); $(docker exec "$run-main" smbd --version)"
if [[ -n "${UNDERDOC_T1_FIXTURES:-}" ]]; then
    UNDERDOC_T1_FIXTURES="$(realpath "$UNDERDOC_T1_FIXTURES")"
    export UNDERDOC_T1_FIXTURES
fi
go test -count=1 -tags t1 -run 'T1' -v -timeout 20m ./internal/share/ || {
    docker logs "$run-main" 2>&1 | tail -40 >&2
    fail "T-1"
}
printf '✓ T-1 LibreOffice\n'
