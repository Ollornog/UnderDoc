#!/usr/bin/env bash
# Tests the working copy against real Samba (ADR-30, docs/samba.md): builds the companion image (deploy/samba/Dockerfile,
# with smbclient for the tests), starts two companions, one for the tenant main and one for acme, each on the directory
# of its tenant only, and runs the tests with the tag samba against them. For the domain (T-42) it starts a domain
# controller of Samba with made-up accounts and a password made for this run, and a third companion for the tenant corp
# that joins it; the start of a companion without a password file, and that the password stands in no log and no
# process, are checked here. Like scripts/check-image.sh it needs Docker and
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
    for t in main acme corp corp-again corp-fresh dc; do dock rm -f -v "$run-$t" >/dev/null 2>&1 || true; done
    dock network rm "$run-net" >/dev/null 2>&1 || true
    dock volume rm "$run-corp-state" "$run-corp-fresh-state" >/dev/null 2>&1 || true
    dock rmi -f "$image" "$image-dc" >/dev/null 2>&1 || true
    # Files the companions made belong to the UIDs of Samba users; a container of the same image takes them away.
    dock run --rm -v "$work:$work" --entrypoint rm debian:trixie-slim@sha256:a72b4433de1cf7d6bae0c4cee74d0547f58601a643b3375f070c51600d63b6b2 -rf "$work/share" >/dev/null 2>&1 || true
    rm -rf "$work"
}
trap cleanup EXIT

dock build --quiet --target test -f deploy/samba/Dockerfile -t "$image" . >/dev/null || fail "the companion image does not build"
dock build --quiet --target test-dc -f deploy/samba/Dockerfile -t "$image-dc" . >/dev/null || fail "the image of the domain controller does not build"

mkdir -p "$work/share/main" "$work/share/acme" "$work/share/corp" "$work/sock/main" "$work/sock/acme" "$work/sock/corp" \
    "$work/bin" "$work/secret"
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

# ---- The domain (T-42): a domain controller of Samba, made up for this run. Its password is made here and never written
# into the repository; every account of the test has it.
password="Ud-$(head -c 18 /dev/urandom | base64 | tr -d '/+=')-9a"
printf '%s' "$password" >"$work/secret/join"
chmod 644 "$work/secret/join" # read by root in the companion; the directory is this run's own
dock network create "$run-net" >/dev/null
# shellcheck disable=SC2016 # expanded by the shell of the domain controller
dock run -d --name "$run-dc" --network "$run-net" --hostname dc1 --label underdoc-samba-test="$run" \
    --cap-drop ALL --cap-add CHOWN --cap-add DAC_OVERRIDE --cap-add FOWNER --cap-add SETUID --cap-add SETGID \
    --cap-add KILL --cap-add NET_BIND_SERVICE --cap-add SYS_ADMIN \
    -v "$work/secret/join:/run/secret/join:ro" --entrypoint sh "$image-dc" -c '
        samba-tool domain provision --realm=EXAMPLE.TEST --domain=EXAMPLE --server-role=dc --dns-backend=SAMBA_INTERNAL \
            --adminpass="$(cat /run/secret/join)" --host-ip="$(hostname -i)" --option="dns forwarder = 127.0.0.1" >/tmp/provision.log 2>&1 \
            || { cat /tmp/provision.log; exit 1; }
        exec samba -F' >/dev/null || fail "the domain controller does not start"
for _ in $(seq 1 120); do
    dock exec "$run-dc" sh -c 'smbclient -L localhost -N >/dev/null 2>&1' && break
    sleep 1
done
dock exec "$run-dc" sh -c 'smbclient -L localhost -N >/dev/null 2>&1' ||
    { dock logs "$run-dc" 2>&1 | tail -20 >&2; fail "the domain controller does not answer"; }
# dora and 4711 are mapped by UnderDoc in the tests, eve is not, daemon is a user of the system.
for u in dora eve daemon 4711; do
    dock exec "$run-dc" sh -c "samba-tool user create '$u' \"\$(cat /run/secret/join)\" >/dev/null" || fail "domain account $u"
done
dc_ip="$(dock inspect -f "{{(index .NetworkSettings.Networks \"$run-net\").IPAddress}}" "$run-dc")"
# The address of the companion as the domain knows it, for Kerberos.
dock exec "$run-dc" sh -c "samba-tool dns add localhost example.test files A 127.0.0.1 -U Administrator --password=\"\$(cat /run/secret/join)\" >/dev/null 2>&1" ||
    fail "the DNS name of the companion"

# corp_companion <name> <state volume> <with password file: yes|no>: each with a socket directory of its own, so a
# companion started for a check never takes the socket of the one the tests talk to.
corp_companion() {
    local join=()
    mkdir -p "$work/sock/$1"
    if [[ "$3" == yes ]]; then
        join=(-e UNDERDOC_SAMBA_DOMAIN_JOIN_USER=Administrator -e UNDERDOC_SAMBA_DOMAIN_JOIN_PASSWORD_FILE=/run/secret/join
            -v "$work/secret/join:/run/secret/join:ro")
    fi
    dock run -d --name "$run-$1" --label underdoc-samba-test="$run" --network "$run-net" --dns "$dc_ip" --hostname files \
        --cap-drop ALL --cap-add CHOWN --cap-add DAC_OVERRIDE --cap-add FOWNER --cap-add SETUID --cap-add SETGID \
        --cap-add KILL --security-opt no-new-privileges --read-only \
        --tmpfs /var/lib/underdoc-samba:mode=0755 --tmpfs /run/samba --tmpfs /tmp \
        -v "$2:/var/lib/underdoc-samba-domain" \
        -e UNDERDOC_UID="$(id -u)" -e UNDERDOC_GID="$(id -g)" -e UNDERDOC_SAMBA_ROOT="$work/share/corp" \
        -e UNDERDOC_SAMBA_NAME=FILES -e UNDERDOC_SAMBA_DOMAIN_REALM=example.test -e UNDERDOC_SAMBA_DOMAIN_WORKGROUP=EXAMPLE \
        "${join[@]}" -v "$work/share/corp:$work/share/corp" -v "$work/sock/$1:/run/underdoc-samba" "$image" >/dev/null
}
healthy() {
    for _ in $(seq 1 120); do
        case "$(dock inspect -f '{{.State.Health.Status}} {{.State.Running}}' "$run-$1")" in
            "healthy true") return 0 ;;
            *" false") return 1 ;;
        esac
        sleep 0.5
    done
    return 1
}
corp_companion corp "$run-corp-state" yes || fail "the companion of corp does not start"
healthy corp || { dock logs "$run-corp" 2>&1 | tail -30 >&2; fail "the companion of corp does not join the domain"; }
dock logs "$run-corp" 2>&1 | grep -q "member of the domain.*joined=now" || fail "the companion of corp did not join now"
printf '#!/usr/bin/env bash\nexec docker exec -i %q smbclient "$@"\n' "$run-corp" >"$work/bin/smbclient-corp"
chmod 755 "$work/bin/smbclient-corp"

# The password stands in no log and in no process of the companion (command line or environment).
if dock logs "$run-corp" 2>&1 | grep -qF -- "$password"; then fail "the password of the join stands in the log of the companion"; fi
# The command lines and environments of every process, read with all capabilities (the companion has none to read those of
# its other processes). That the reading sees them is checked first: the realm of the companion stands in the
# environment of winbindd, a child of the helper.
# shellcheck disable=SC2016 # expanded inside the container
procs="$(dock exec --privileged "$run-corp" sh -c 'for p in /proc/[0-9]*; do cat "$p/cmdline" "$p/environ" 2>/dev/null; echo; done' | tr '\0' '\n')"
if ! grep -q '^UNDERDOC_SAMBA_DOMAIN_REALM=example.test$' <<<"$procs" || ! grep -q '^winbindd$' <<<"$procs"; then
    fail "the processes of the companion cannot be read: the check of the password would see nothing"
fi
if grep -qF -- "$password" <<<"$procs"; then fail "the password of the join stands in a process of the companion"; fi
if dock exec "$run-corp" sh -c 'ps -eo args' | grep -qF -- "$password"; then fail "the password of the join stands in the process list"; fi

# Joined before: a new container on the same volume needs no password file.
corp_companion corp-again "$run-corp-state" no || fail "the second companion of corp does not start"
healthy corp-again || { dock logs "$run-corp-again" 2>&1 | tail -20 >&2; fail "a companion on the volume of a joined one does not start without the password file"; }
dock logs "$run-corp-again" 2>&1 | grep -q "joined=before" || fail "the second companion joined again instead of using the volume"
dock rm -f -v "$run-corp-again" >/dev/null
# Not joined and no password file: the companion ends, loudly, and never gets healthy.
corp_companion corp-fresh "$run-corp-fresh-state" no || fail "the companion without a password file does not start at all"
if healthy corp-fresh; then fail "a companion that is not joined started without a password file"; fi
dock logs "$run-corp-fresh" 2>&1 | grep -q "no UNDERDOC_SAMBA_DOMAIN_JOIN_USER and _PASSWORD_FILE" ||
    { dock logs "$run-corp-fresh" 2>&1 | tail -10 >&2; fail "the companion without a password file does not say why it ends"; }
dock rm -f -v "$run-corp-fresh" >/dev/null

export UNDERDOC_TEST_SAMBA_DIR="$work/share"
export UNDERDOC_TEST_SAMBA_SOCKETS="main=$work/sock/main/helper.sock,acme=$work/sock/acme/helper.sock"
export UNDERDOC_TEST_SAMBA_CLIENTS="main=$work/bin/smbclient-main,acme=$work/bin/smbclient-acme"
export UNDERDOC_TEST_SAMBA_DOMAIN="corp=$work/sock/corp/helper.sock"
export UNDERDOC_TEST_SAMBA_DOMAIN_CLIENT="$work/bin/smbclient-corp"
export UNDERDOC_TEST_SAMBA_DOMAIN_PASSWORD_FILE="$work/secret/join"
go test -count=1 -tags samba -run 'Samba' ./internal/share/ || {
    for t in main acme corp; do dock logs "$run-$t" 2>&1 | tail -30 >&2; done
    fail "the tests against Samba"
}
printf '✓ Samba\n'
