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
- **Firmenrechte:** Nutzer, Gruppen, OIDC und SAML, dazu öffentliches Teilen.
- **Später:** Dokumente nebeneinander, ein Posteingang, ein Kalender, Formulare, Reviews über Git, ein Website-Baukasten, gute
  Suche und optionale KI über die eigenen Dokumente.

Alles beginnt lesend: Bearbeitet wird an der Quelle, der Web-Editor kommt später. Siehe die
[Meilensteine](../backlog/README.md).

## Installation

Releases gibt es noch nicht. Gebaut wird aus dem Quelltext mit Go oder Docker, wie unten gezeigt.

## Ausprobieren

UnderDoc hat keine eigene Anmeldung und lauscht deshalb standardmäßig nur auf Loopback. Bevor andere darauf zugreifen können, gehört eine Anmeldeschranke davor.

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

**Export und Import** brauchen die Datenbank (`UNDERDOC_DATABASE_URL`). `underdoc export <ordner>` schreibt jede Sammlung als
normale Dateien: Ordner, Markdown-Dateien mit Kopfbereich und vor jedem Block ein unsichtbarer Kommentar, der seine ID hält.
`underdoc import --owner <login> <ordner> <titel>` liest so einen Baum, oder jeden Ordner mit Markdown-Dateien, in eine neue
Sammlung ein. Aufbau und Regeln stehen (englisch) in [docs/export.md](../docs/export.md).

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
