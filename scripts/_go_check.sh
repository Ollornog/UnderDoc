#!/usr/bin/env bash
# Der Go-Teil des Tors — aus dem geteilten Kit (repokit), nicht von Hand ändern.
#
#   scripts/_go_check.sh            gofmt, go mod tidy, go vet, staticcheck, govulncheck, go test -race
#   scripts/_go_check.sh --fast     ohne govulncheck (Netz) und ohne Race-Detektor; kein Ersatz fürs Tor
#
# Umgebung:
#   GO_PRUEF_TAGS   zusätzliche Build-Tags, je Tag ein eigener vet-/staticcheck-Durchgang (z. B. "embedsite browser"),
#                   damit Code hinter einem Tag nicht ungeprüft bleibt
#   CI_KERNE        Pakete gleichzeitig (Vorgabe 2) — die Zahl des Runners, nicht GOMAXPROCS
#
# Werkzeuge: `.github/audit/go.mod` pinnt staticcheck und govulncheck (eigenes Modul, damit ihre Abhängigkeiten
# nicht in den Modulgraphen des Programms geraten; Dependabot hebt es, Ökosystem gomod, directory /.github/audit).
# Liegt GENAU diese Fassung schon im PATH (ci-go), wird sie genommen, sonst aus dem Werkzeug-Modul gebaut.
# Fehlt beides, ist die Suite rot — kein Überspringen.
set -euo pipefail
cd "$(dirname "$0")/.."

FAST=0
[[ "${1:-}" == "--fast" ]] && FAST=1

step() { printf '\n\033[1m▸ %s\033[0m\n' "$1"; }
fail() { printf '\n\033[31m✗ %s\033[0m\n' "$1" >&2; exit 1; }

command -v go >/dev/null || fail "Go-Quellen vorhanden, aber keine Go-Toolchain im PATH"
[[ -f go.mod ]] || fail "kein go.mod in der Wurzel"
WERKZEUG_MOD=".github/audit/go.mod"
[[ -f "$WERKZEUG_MOD" ]] || fail "$WERKZEUG_MOD fehlt — es pinnt staticcheck und govulncheck (siehe repokit README, Go)"

KERNE="${CI_KERNE:-2}"
[[ "$KERNE" =~ ^[1-9][0-9]*$ ]] || fail "CI_KERNE=$KERNE — erwartet eine ganze Zahl ≥ 1"
read -r -a TAGS <<<"${GO_PRUEF_TAGS:-}"

WZ="$(mktemp -d)"
trap 'rm -rf "$WZ"' EXIT

# werkzeug <name> <modul> <paket> — gibt den Pfad eines Binarys in genau der gepinnten Fassung aus.
werkzeug() {
    local name="$1" modul="$2" paket="$3" soll ist bin
    soll="$(go -C .github/audit list -m -f '{{.Version}}' "$modul")" || fail "$modul nicht in $WERKZEUG_MOD"
    if bin="$(command -v "$name")"; then
        ist="$(go version -m "$bin" 2>/dev/null | awk -v m="$modul" '$1=="mod" && $2==m {print $3}')"
        if [[ "$ist" == "$soll" ]]; then
            printf '%s' "$bin"
            return
        fi
    fi
    go -C .github/audit build -o "$WZ/$name" "$paket" || fail "$name $soll bauen"
    printf '%s' "$WZ/$name"
}

step "gofmt — Formatierung"
UNFORMATIERT="$(gofmt -l . 2>&1 || true)"
[[ -z "$UNFORMATIERT" ]] || fail "nicht gofmt-konform:"$'\n'"$UNFORMATIERT"
echo "  alle Dateien formatiert"

step "go mod tidy — go.mod/go.sum stimmen (Programm und Werkzeuge)"
go mod tidy -diff || fail "go.mod/go.sum nicht aufgeräumt (go mod tidy)"
go -C .github/audit mod tidy -diff || fail "$WERKZEUG_MOD nicht aufgeräumt (go -C .github/audit mod tidy)"
echo "  aufgeräumt"

step "go vet"
go vet -p "$KERNE" ./... || fail "go vet"
for t in "${TAGS[@]}"; do go vet -p "$KERNE" -tags "$t" ./... || fail "go vet -tags $t"; done

STATICCHECK="$(werkzeug staticcheck honnef.co/go/tools honnef.co/go/tools/cmd/staticcheck)"
step "staticcheck ($("$STATICCHECK" -version | head -1))"
"$STATICCHECK" ./... || fail "staticcheck"
for t in "${TAGS[@]}"; do "$STATICCHECK" -tags "$t" ./... || fail "staticcheck -tags $t"; done

if [[ $FAST -eq 0 ]]; then
    # Prüft Module UND Standardbibliothek der laufenden Go-Fassung; rot nur bei erreichbarem Code. Braucht Netz
    # (vuln.go.dev). Ohne Netz ist das ein Fehler, kein Überspringen.
    GOVULNCHECK="$(werkzeug govulncheck golang.org/x/vuln golang.org/x/vuln/cmd/govulncheck)"
    step "govulncheck ($("$GOVULNCHECK" -version 2>/dev/null | grep -m1 -i 'scanner' || echo gepinnt))"
    n="$(go list ./... | wc -l)"
    [[ "$n" -gt 0 ]] || fail "keine Pakete — govulncheck prüfte nichts"
    "$GOVULNCHECK" ./... || fail "govulncheck"
    for t in "${TAGS[@]}"; do "$GOVULNCHECK" -tags "$t" ./... || fail "govulncheck -tags $t"; done
fi

# -count=1: kein Test-Cache, sonst prüft der zweite Lauf nichts.
if [[ $FAST -eq 1 ]]; then
    step "go test (kurz, --fast)"
    go test -p "$KERNE" -count=1 -short ./... || fail "go test -short"
    printf '\n\033[33m! --fast: govulncheck und Race-Detektor übersprungen. Vor dem Push ohne --fast.\033[0m\n'
else
    step "go test — mit Race-Detektor"
    go test -p "$KERNE" -count=1 -race ./... || fail "go test -race"
fi
