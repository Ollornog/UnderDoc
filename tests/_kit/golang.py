"""Go-Repos: Werkzeug-Pins und Dependabot-Abdeckung — die Go-Variante des Kits (seit 0.38.0).

Diese Datei wird von `repokit sync --go` hierher kopiert — nicht von Hand ändern. Sie liegt nur in Repos,
die die Go-Variante gewählt haben (`scripts/_go_check.sh` ist da); dort fordert
`hygiene.pruefe_kit_prueffunktionen_gerufen` ihren Aufruf, in allen anderen Repos nicht.

WARUM (PO 2026-10-22, „Standard für schlichte Webserver in Go“): Der Go-Teil des Tors (`scripts/_go_check.sh`)
baut seine Prüfwerkzeuge aus `.github/audit/go.mod`. Fehlt dort ein Werkzeug, bricht er ab; fehlt der
Dependabot-Eintrag für dieses Modul, veraltet der Pin still. Ebenso ein Dockerfile ohne `docker`-Eintrag:
das Basis-Abbild bliebe auf ewig beim Digest vom Tag des Anlegens. Beides ist im Repo ablesbar, also hier.

stdlib-only, kein YAML-Parser: gelesen werden nur `package-ecosystem`, `directory` und `default-days`.
"""
from __future__ import annotations

import os
import re

from .hygiene import mit_fallzahl, zaehle_fall, zeilen_wie_grep

WERKZEUG_MOD = ".github/audit/go.mod"
# Werkzeug -> Paket, wie _go_check.sh es baut.
WERKZEUGE = {
    "staticcheck": "honnef.co/go/tools/cmd/staticcheck",
    "govulncheck": "golang.org/x/vuln/cmd/govulncheck",
}


def _lies(root: str, rel: str) -> str | None:
    try:
        with open(os.path.join(root, rel), encoding="utf-8") as fh:
            return fh.read()
    except OSError:
        return None


def tool_direktiven(go_mod: str) -> set[str]:
    """Die Pakete der `tool`-Direktiven — Einzelzeile UND Block (`tool ( … )`)."""
    pakete: set[str] = set()
    im_block = False
    for zeile in zeilen_wie_grep(go_mod):
        z = zeile.split("//", 1)[0].strip()
        if im_block:
            if z == ")":
                im_block = False
            elif z:
                pakete.add(z)
            continue
        if re.fullmatch(r"tool\s*\(", z):
            im_block = True
        elif m := re.fullmatch(r"tool\s+(\S+)", z):
            pakete.add(m.group(1))
    return pakete


def pruefe_go_werkzeuge(root: str) -> list[str]:
    """`.github/audit/go.mod` pinnt jedes Werkzeug, das `scripts/_go_check.sh` baut, und `go.sum` liegt daneben."""
    text = _lies(root, WERKZEUG_MOD)
    if text is None:
        return [f"{WERKZEUG_MOD} fehlt — es pinnt staticcheck und govulncheck für scripts/_go_check.sh"]
    treffer = []
    direktiven = tool_direktiven(text)
    for name, paket in sorted(WERKZEUGE.items()):
        zaehle_fall()
        if paket not in direktiven:
            treffer.append(f"{WERKZEUG_MOD}: keine tool-Direktive für {name} ({paket}) — "
                           f"`go -C .github/audit get -tool {paket}@<version>`")
    if _lies(root, ".github/audit/go.sum") is None:
        treffer.append(".github/audit/go.sum fehlt — ohne Prüfsummen ist der Pin keiner")
    return treffer


def dependabot_eintraege(text: str) -> list[dict[str, str]]:
    """Je `- package-ecosystem:`-Block: Ökosystem, Verzeichnis, default-days (Text, wie geschrieben)."""
    eintraege: list[dict[str, str]] = []
    aktuell: dict[str, str] | None = None
    for zeile in zeilen_wie_grep(text):
        z = zeile.split("#", 1)[0].rstrip()
        if m := re.match(r"\s*-\s*package-ecosystem:\s*[\"']?([\w-]+)", z):
            aktuell = {"oeko": m.group(1)}
            eintraege.append(aktuell)
        elif aktuell is not None and (m := re.match(r"\s+directory:\s*[\"']?([^\"'\s]+)", z)):
            aktuell["verzeichnis"] = m.group(1).rstrip("/") or "/"
        elif aktuell is not None and (m := re.match(r"\s+default-days:\s*(\d+)", z)):
            aktuell["tage"] = m.group(1)
    return eintraege


def pruefe_dependabot_go(root: str, dateien: list[str]) -> list[str]:
    """Jedes go.mod und jedes Dockerfile ist von Dependabot abgedeckt, mit Wartezeit.

    - je Verzeichnis mit `go.mod` (auch `.github/audit`) ein Eintrag `gomod`,
    - je Verzeichnis mit `Dockerfile` ein Eintrag `docker`,
    - jeder dieser Einträge mit `cooldown: default-days` (PO 2026-09-28: drei Tage).
    """
    text = _lies(root, ".github/dependabot.yml")
    if text is None:
        return [".github/dependabot.yml fehlt"]
    eintraege = dependabot_eintraege(text)
    treffer = []
    soll: list[tuple[str, str, str]] = []
    for rel in sorted(dateien):
        name = os.path.basename(rel)
        verz = "/" + os.path.dirname(rel) if os.path.dirname(rel) else "/"
        if name == "go.mod" and "/testdata/" not in "/" + rel:
            soll.append(("gomod", verz, rel))
        elif name == "Dockerfile":
            soll.append(("docker", verz, rel))
    for oeko, verz, rel in soll:
        zaehle_fall()
        passend = [e for e in eintraege if e["oeko"] == oeko and e.get("verzeichnis") == verz]
        if not passend:
            treffer.append(f"{rel}: kein Dependabot-Eintrag `{oeko}` für directory {verz}")
        elif not all(e.get("tage") for e in passend):
            treffer.append(f"{rel}: Dependabot-Eintrag `{oeko}` {verz} ohne cooldown default-days")
    return treffer


def pruefe_go_check_verdrahtet(root: str) -> list[str]:
    """`scripts/check.sh` ruft den Go-Teil des Tors — eine gelieferte Prüfung, die niemand ruft, ist keine."""
    zaehle_fall()
    text = _lies(root, "scripts/check.sh")
    if text is None:
        return ["scripts/check.sh fehlt"]
    aufrufe = [z for z in zeilen_wie_grep(text)
               if "scripts/_go_check.sh" in z and not z.lstrip().startswith("#")]
    if not aufrufe:
        return ["scripts/check.sh ruft scripts/_go_check.sh nicht auf"]
    return []


pruefe_go_werkzeuge = mit_fallzahl("golang.pruefe_go_werkzeuge", pruefe_go_werkzeuge)
pruefe_dependabot_go = mit_fallzahl("golang.pruefe_dependabot_go", pruefe_dependabot_go)
pruefe_go_check_verdrahtet = mit_fallzahl("golang.pruefe_go_check_verdrahtet", pruefe_go_check_verdrahtet)
