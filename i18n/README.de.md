<p align="center"><img src="../docs/logo.svg" width="250" height="250" alt="UnderDoc-Logo"></p>
<h1 align="center">UnderDoc</h1>

<p align="center"><a href="../README.md">English</a> · <b>Deutsch</b></p>

**Das Wissen deines Teams an einem Ort, ohne jemandem Excel wegzunehmen.**

> **Stand: früh.** Ein Ordner mit Markdown-Dateien funktioniert schon als Wiki zum Lesen (M-1). Alles Weitere unten ist noch
> Plan, siehe [`docs/concept.md`](../docs/concept.md) und [`backlog/`](../backlog/).

## Warum

Gemeinsame Doku landet oft in Excel-Dateien, die niemand findet, in einem Wiki, das niemand pflegt, oder
in beidem mit unterschiedlichen Zahlen. UnderDoc ist ein selbst gehostetes Wiki mit einer Regel: **Jede
Angabe hat genau eine Quelle.**

## Was es können wird

- **Einfache Markdown-Seiten**, in Ordnern angeordnet, sichtbar als Wiki, als private Notizen und als Dateibaum.
- **Dokumente mit Eigentümern,** Bearbeitern, Kommentatoren und Betrachtern. Verschieben ändert, wo sie stehen, nicht was sie sind.
- **Excel bleibt Excel.** Dateien liegen auf einem Windows-Share und öffnen sich wie gewohnt in Excel oder
  LibreOffice. Das Web zeigt dieselbe Datei.
- **Einbinden statt kopieren.** Tabellen, Datenbank-Ansichten, Bilder, Snippets und Diagramme kommen aus ihrer Quelle.
- **Werte als Variablen.** `{{tower.height}}` auf zehn Seiten schreiben und einmal ändern.
- **Native Listen** mit Verknüpfungen untereinander, für Kunden, Kontakte und Ähnliches.
- **Externe Quellen** mit Zwischenspeicher, dazu Exporte, geplante Berichte und eine API für andere Systeme.
- **Mermaid-Diagramme**, auch mit Werten und Snippets.
- **Firmenrechte:** Nutzer, Gruppen und Rollen aus der Anmeldeschranke (die OIDC, SAML und LDAP spricht), dazu öffentliches Teilen.
- **Später:** Dokumente nebeneinander, ein Posteingang, ein Kalender, Formulare, Reviews über Git, ein Website-Baukasten, gute
  Suche und optionale KI über die eigenen Dokumente.

Alles beginnt lesend: Bearbeitet wird an der Quelle, der Web-Editor kommt später. Siehe die
[Meilensteine](../backlog/README.md).

## Installation

Releases gibt es noch nicht. Jeder Stand auf `main` erscheint als Container-Abbild für amd64 und arm64,
`ghcr.io/ollornog/underdoc:main-<erste 12 Hex-Zeichen des Commits>`; festhalten mit Tag und Digest. Oder aus dem
Quelltext mit Go oder Docker bauen, wie unten gezeigt.

## Ausprobieren

UnderDoc hat keine eigene Anmeldung und lauscht deshalb standardmäßig nur auf Loopback. Angemeldet wird nur über die Anmeldeschranke davor, [TinySesam](https://github.com/Ollornog/TinySesam), deren Go-Client jede Anfrage prüft; eine Person wird über `Remote-Id` erkannt, Administratoren tragen die Rolle der App (`underdoc-admin`), Bereiche auf oberster Ebene legen nur sie und Inhaber von `underdoc-spaces` an (`UNDERDOC_SPACES_ROLE`), eine Anfrage ohne bekanntes `Remote-Kind` wird abgelehnt, und ein Dienstkonto kommt nur herein, wenn seine ID in `UNDERDOC_SERVICE_ACCOUNTS` steht; es gilt dann immer als Agent und gehört nie zu „alle Angemeldeten“ ([ADR-32](../backlog/ADR-32-sign-in-only-through-the-gate.md)). Gruppen werden archiviert statt gelöscht; endgültig aus dem Papierkorb löschen nur Besitzer oder Administratoren im Browser, mit eingetipptem Namen.

```bash
go run ./cmd/underdoc serve ~/notes     # http://127.0.0.1:8080

docker build -t underdoc .
docker run --rm --read-only --cap-drop ALL --security-opt no-new-privileges \
  -p 127.0.0.1:8080:8080 -v ~/notes:/data:ro underdoc
```

| Einstellung | Bedeutung | Vorgabe |
|---|---|---|
| `UNDERDOC_DIR` | Der Ordner, oder das Argument von `serve` | keine |
| `UNDERDOC_ADDR` | Adresse des Wikis | `127.0.0.1:8080`, im Container `:8080` |
| `UNDERDOC_HEALTH_ADDR` | Adresse von `GET /healthz` | `127.0.0.1:8081` |
| `UNDERDOC_SCAN_INTERVAL` | Wie oft der Ordner geprüft wird | `2s` |
| `UNDERDOC_DEV_UI` | `1` zeigt alle Bausteine unter `/dev/ui` | aus |

**Die Suche** (ADR-24) läuft über einen Suchdienst, als Vorgabe Meilisearch, wahlweise OpenSearch: Fehlercodes werden genau so gefunden, wie
sie geschrieben sind, und zuerst, danach Titel vor Überschriften vor Text, mit deutschen Komposita und Tippfehlern, und ein
Treffer öffnet an seinem Abschnitt. Strg+K sucht von überall. Ohne Suchdienst werden nur Titel durchsucht. Einstellungen,
Rangfolge und die Messung an einem deutschen Testkorpus stehen (englisch) in [docs/search.md](../docs/search.md). Die
Dienste neben UnderDoc (Meilisearch, OpenSearch, Mailpit, Gotenberg) startet [`deploy/compose.yml`](../deploy/compose.yml),
siehe (englisch) [docs/services.md](../docs/services.md). Ein neues Meilisearch-Abbild übernimmt die Datenbank des
alten selbst (`MEILI_UPGRADE_DB`); das ist nicht atomar, also vorher eine Sicherung des Volumes anlegen
([Updating Meilisearch](../docs/services.md#updating-meilisearch)).

**Export und Import** brauchen die Datenbank (`UNDERDOC_DATABASE_URL`). `underdoc export <ordner>` schreibt jede Sammlung als
normale Dateien: Ordner, Markdown-Dateien mit Kopfbereich und vor jedem Block ein unsichtbarer Kommentar, der seine ID hält.
`underdoc import --owner <login> <ordner> <titel>` liest so einen Baum, oder jeden Ordner mit Markdown-Dateien, in eine neue
Sammlung ein. Aufbau und Regeln stehen (englisch) in [docs/export.md](../docs/export.md).

**Dateien** brauchen die Datenbank und `UNDERDOC_STORE_DIR`: die Ansicht Dateien hat den Ordner der Firma, einen Ordner
je Nutzer und Sammlungen, mit Uploads, die nach einem Abbruch weiterlaufen, Versionen, Kontingenten und einem Papierkorb.
Jede Version liegt einmal in einem Inhaltsspeicher, benannt nach ihrem Hash, und ein Export nimmt die Dateien mit. Wie
das arbeitet, steht (englisch) in [docs/files.md](../docs/files.md).

**Seitenarten** sagen, für wen eine Seite ist: Struktur und Datensätze, Doku und Referenz für Agenten oder Doku für
Menschen (Anleitung, Erklärung, Tutorial). Jedes Schreiben wird gegen die Regeln der Art geprüft; Fehler blockieren,
Warnungen kommen mit einem Hinweis zur Behebung zurück, etwa „embed instead of copying“. Agenten lesen die Rohseite mit
ihrem Kopfbereich. Was ein Agent an Doku für Menschen ändert, trägt „written by AI, not checked“, bis ein Mensch den
Diff prüft oder die Änderung zurücknimmt (englisch): [docs/page-kinds.md](../docs/page-kinds.md).

**Agenten über MCP** erreichen die Seiten unter `/mcp` (ADR-34): angemeldet über das Gate mit OAuth, immer als Agent
ihrer Person, also bleibt ein Space mit „keine Agenten“ zu, und Rechte, Freigabelinks, endgültiges Löschen und
Passwörter bleiben außer Reichweite. Ein kleiner Kern an Werkzeugen: `context` zum Anfangen, `read` für Seiten und
Abschnitte mit der Version, gegen die geschrieben wird, `used_in`, eine `search` über Abschnitte von Seiten und Seiten
von PDF, `write` und `replace` gegen diese Version mit Probelauf, Diff und Prüfung in jeder Antwort, Front Matter, eine
neue Seite, `move`, `trash`, und Dateien, Seite für Seite gelesen und über einen Einmal-Link hochgeladen. Die
Konventionen sind eine Regelseite im Wiki: [i18n/docs/mcp.de.md](docs/mcp.de.md).

**Der Web-Editor** schreibt eine Seite der Datenbank in einem Editor mit Live-Vorschau: Markdown, das abseits des Cursors
gerendert aussieht, eine Leiste über der Seite, `/` für die Arten von Blöcken und Speichern beim Tippen. Er wird mit
Node aus `web/editor` gebaut, und das gebaute Paket liegt im Repository, also braucht der Bau von UnderDoc nur Go. Wie
er arbeitet und wie man ihn baut: [docs/editor.md](../docs/editor.md).

**Seiten schreiben:**

- Eine Seite ist eine `.md`-Datei, geschrieben in einem beliebigen Texteditor. Versteckte Dateien und symbolische Links werden übersprungen.
- Eine feste Adresse, die ein Verschieben übersteht, gibt der Kopfbereich der Seite:

  ```markdown
  ---
  id: netzplan
  title: Netzplan
  ---
  ```

- Verlinken geht mit `[[netzplan]]`, `[[Netzplan]]`, `[[netzplan|anderer Text]]` oder `[[netzplan#Überschrift]]`. Links über ID oder
  Titel funktionieren auch nach dem Verschieben. Normale relative Links gehen ebenfalls, hängen aber am Pfad.
- Bilder neben einer Seite funktionieren wie gewohnt mit `![Karte](karte.png)`. Bilder von anderen Hosts werden zu Links, denn Seiten laden nichts von Dritten.
- `index.md` oder `README.md` im obersten Ordner ist die Startseite.
- Was kaputt ist, zeigt die Seite selbst und die Liste unter **Problems**.

## Lizenz

[MIT](../LICENSE).
