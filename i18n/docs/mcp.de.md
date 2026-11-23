# UnderDoc als MCP-Server

<p><a href="../../docs/mcp.md">English</a> · <b>Deutsch</b></p>

Agenten wie Coding-Assistenten erreichen die Seiten und Dateien von UnderDoc über das Model Context Protocol (ADR-34,
T-21). Sie handeln immer für eine Person, mit deren Rechten, und immer als Agent (ADR-36). UnderDoc braucht dafür kein
KI-Modell (M-13).

## Der Endpunkt

UnderDoc antwortet auf MCP unter **`/mcp`**, über den Transport Streamable HTTP, gebaut auf dem offiziellen Go-SDK
(`github.com/modelcontextprotocol/go-sdk`). Der Server ist zustandslos: Jeder POST ist ein ganzer Austausch und wird als
JSON beantwortet. Zwischen Anfragen hält er keine Sitzung, also wird jeder Aufruf mit den Rechten des Augenblicks neu
entschieden. Er bietet nur Werkzeuge an, keine Ressourcen und keine Prompts.

## Anmeldung

UnderDoc enthält keinen OAuth-Code. Das Gate davor (ADR-32, ADR-35) ist der OAuth-2.1-Autorisierungsserver für
MCP-Clients:

- Der Betreiber trägt `https://<host>/mcp` als OAuth-Ressource in die Konfiguration des Gates ein. Ein Token dafür gilt
  für diesen Pfad und alles darunter.
- Der Proxy des Gates leitet `/.well-known/oauth-protected-resource` des Hosts von UnderDoc an das Gate, das die
  Metadaten schreibt (RFC 9728). UnderDoc hat dafür keinen Handler.
- Eine Anfrage ohne gültiges Token bekommt vom Gate `401` mit `WWW-Authenticate: Bearer resource_metadata=…`. Ein Client
  meldet sich dann über das Gate an und registriert sich dort.
- Mit gültigem Token reicht das Gate die Anfrage mit denselben `Remote-*`-Headern weiter wie für einen Browser, mit
  `Remote-Auth: oauth` und `Remote-Kind: user`. Der Go-Client des Gates prüft sie wie bei jeder Anfrage
  ([sign-in.md](../../docs/sign-in.md#the-gate)).

Eine Anfrage, die `/mcp` ohne das Gate erreicht, bekommt `401`, eine ohne `Remote-Kind` `403`, wie überall.

Um einen Client anzubinden, gib ihm die Adresse des Endpunkts, zum Beispiel mit Claude Code:

```bash
claude mcp add --transport http underdoc https://wiki.example.org/mcp
```

Der Client findet das Gate über die Metadaten und öffnet dessen Anmeldung im Browser.

## Immer ein Agent

Jeder Aufruf über `/mcp` ist der eines Agenten (`account.User.MCP`), auch wenn eine Browser-Sitzung des Gates ihn
geschickt hat. Die Verbindung jeder Anfrage zur Datenbank erfährt das, also hält die eine Rechteprüfung
(`object_decide`) die Obergrenze der Gruppe Agenten ein ([rights.md](../../docs/rights.md)):

- keine Rechte verwalten, keine Freigabelinks, nichts endgültig löschen, keine Passwörter;
- ein Space mit „keine Agenten“ ist zu, im Baum, beim Lesen und in der Suche;
- jede Person kann ihre eigenen Agenten weiter begrenzen, zum Beispiel aufs Lesen;
- der Agent eines Administrators verwaltet nichts.

Ein Browser kann von einer Seite einer anderen Website keine Anfrage an `/mcp` schicken: So eine Anfrage
(`Sec-Fetch-Site: cross-site` oder ein `Origin` eines anderen Hosts) wird mit `403` abgelehnt, denn das Gate meldet
Browser mit einem Cookie an. Ein Programm schickt beides nicht und kommt durch.

Jede Seite, die ein Agent liest, wird wie im Browser als geöffnet protokolliert (ADR-11).

## Adressen

Die Werkzeuge benennen, was sie lesen, mit einem Schema, damit andere Quellen später ihres ergänzen (T-31):

| Adresse | Bezeichnet |
|---|---|
| `underdoc:<id>` | ein Objekt der Datenbank: eine Seite, einen Ordner, eine Sammlung oder eine Datei |
| `underdoc:<id>#b3` | einen Block oder den Abschnitt, den ein Überschriftsblock eröffnet |
| `underdoc:<id>#b3.t2` | einen Teil (ADR-22) |
| `underdoc:<id>#page=2` | eine Seite eines PDF |
| `underdoc:folder/<id>` | eine Seite der Ordnerquelle, nur lesend |

Ein Werkzeug nimmt auch die bloße Adresse (`<id>#b3`) und einen Link des Arbeitsbereichs (`/o/<id>#b3`, mit oder ohne
Host). Eine Adresse wird immer nur über ihre ID nachgeschlagen: Nichts davon erreicht einen Pfad auf einer Platte.

## Werkzeuge

| Werkzeug | Tut |
|---|---|
| `context` | Hier anfangen: für wen der Agent handelt und was er darf, die Regelseite, offene Hinweise, der Baum bis zu einer Tiefe (2, wenn nicht anders verlangt, höchstens 4, höchstens 200 Einträge; `under` für einen Teil davon); archivierte Bereiche nur mit `archived` |
| `read` | Eine bis fünf Seiten als Markdown mit Front Matter, der Markierung vor jedem Block und der Version, gegen die geschrieben wird; oder ein Block oder Abschnitt (`#b3`); eine Seite der Ordnerquelle als Gliederung |
| `used_in` | Die Seiten, die eine Seite, einen Block, Abschnitt oder Teil einbetten |
| `search` | Eine Suche über die Abschnitte von Seiten und die Seiten von PDF und anderen Dateien: eine Zeile je Treffer mit Adresse, Abschnitt und Auszug, höchstens 20 Treffer, die Gesamtzahl am Ende; Fehlercodes exakt; archivierte Bereiche nur mit `archived` oder über `in` benannt |
| `write` | Eine Seite oder den Abschnitt, den ein Block eröffnet (`#b3`), als Markdown mit den Markierungen, die read geliefert hat, gegen die gelesene Version (`base`); `dry_run` liefert Prüfung und Diff ohne zu speichern |
| `replace` | Eine Passage, die einmal in der Seite steht, gegen `base`; `dry_run` wie bei `write` |
| `set_front_matter` | Titel, Tags oder Art einer Seite |
| `create_page` | Eine Seite in einer Sammlung, einem Ordner oder einer Seite, mit Art und Text |
| `move` | Ein Objekt in eine Sammlung, einen Ordner oder eine Seite, oder einen Block in eine andere Seite |
| `trash` | Ein Objekt mit allem darunter in den Papierkorb, aus dem eine Person es zurückholen kann |
| `list_files` | Die Wurzeln der Ansicht Dateien, was in einem Ordner liegt, oder die Anhänge einer Seite (Modul Dateien) |
| `read_file_page` | Den Text einer Datei Seite für Seite (`#page=2`) (Modul Dateien) |
| `upload_link` | Einen Einmal-Link, um eine Datei in einen Ordner, als neue Version einer Datei oder an eine Seite hochzuladen (Modul Dateien) |

Antworten sind knapp (ADR-34): Eine Liste hat höchstens 20 Zeilen, eine Antwort höchstens 100 KB. Eine größere Seite,
oder eine größer als ihr Anteil, wenn mehrere auf einmal gelesen werden, kommt als Gliederung mit der Adresse jedes
Abschnitts. Fehler sind Antworten, die der Agent liest: „Not found“ verrät nie, ob etwas existiert, das die Person nicht
sehen darf.

### Die Regelseite

Die Konventionen einer Instanz sind eine Seite im Wiki (ADR-33, ADR-34): die älteste Seite der Art `agent-notes` mit dem
Tag `rules`, die die Person lesen darf. `context` nennt sie; die Anweisungen des Servers sagen, sie vor dem Schreiben zu
lesen. Menschen schreiben und ändern sie im Browser wie jede Seite.

### Anweisungen

Die Anweisungen des Servers sind kurz (deutlich unter den 4.096 Zeichen, die Clients nehmen): die Grenzen des Agenten,
die Adressen, die Zielgruppen der Seitenarten, die Grenzen der Antworten und wie gearbeitet wird. Alles andere steht auf
der Regelseite.

## Schreiben

Eine Seite wird als Markdown gelesen: Front Matter, dann jeder Block nach einer Markierungszeile `<!-- block b3 -->`, und
die Version der Seite, die neueste Änderung ihrer Blöcke. Ein Schreiben schickt sie in derselben Form zurück, mit dieser
Version als `base`:

- Ein Block mit der Markierung eines Blocks des geschriebenen Teils behält seine ID; sein Text wird, wenn geändert, eine
  Version.
- `<!-- block -->`, eine Markierung ohne ID, beginnt einen neuen Block; Text vor der ersten Markierung oder ein Text ganz
  ohne Markierung wird vor seinen Überschriften in neue Blöcke geteilt. Eine ID, die nicht im geschriebenen Teil steht,
  wird nie übernommen: Dieser Block wird neu angelegt.
- Ein Block des geschriebenen Teils, der fehlt, wird gelöscht. Was eine andere Seite einbettet, wird nicht gelöscht: Die
  Antwort nennt die Seiten, und eine Person entscheidet im Browser, was aus den Verwendungen wird, oder der Agent
  verschiebt den Block mit `move`, dem sie folgen.
- Front Matter in einem Schreiben wird nicht übernommen; `set_front_matter` setzt Titel, Tags und Art.
- **Konflikt:** Eine Seite, die sich seit dem Lesen ihrer Version geändert hat, wird abgelehnt, „page changed since you
  read it“, mit beiden Versionen. Ein Schreiben ohne `base` wird ebenfalls abgelehnt. Einen Block innerhalb einer Seite
  zu verschieben macht keine Version und ist kein Konflikt.
- **Probelauf:** Das ganze Schreiben läuft in einer Transaktion, die danach zurückgenommen wird; die Antwort ist
  dieselbe, gespeichert wird nichts.

Jede Antwort enthält den Diff der Blöcke und die Prüfung der Seite ([page-kinds.md](../../docs/page-kinds.md)): Fehler
blockieren das Schreiben, Warnungen kommen mit ihrer Behebung. Für Agenten zählen diese am meisten: `LINK` (ein Link
ins Leere), `UNLINKED-FILE` (eine Datei, auf die nichts verweist), `PAGE-SIZE` (eine Seite über 25 KB), `EMBED`, `COPY`
und die Regeln der Art. Dieselbe Prüfung läuft im Browser.

Was ein Agent schreibt, ist die Version eines Agenten (ADR-33): Auf Doku für Menschen tragen die Blöcke, die er geändert
hat, „written by AI, not checked“, bis eine Person sie prüft, und was er herausgenommen hat, steht unter der Seite. Jede
Änderung wird als Änderung der Person protokolliert, einmal je Stunde und Objekt wie im Browser (ADR-11); die Versionen
halten jede einzelne fest.

## Struktur

- `create_page` legt eine Seite in einer Sammlung, einem Ordner oder einer Seite an; einen Space oben legt eine Person
  an.
- `move` behält die Adresse eines Objekts, also führt jeder Link weiter dorthin. Ein Block, der in eine andere Seite
  verschoben wird, hinterlässt eine Weiterleitung: Seine alte Adresse führt zur neuen, und was ihn eingebettet hat, folgt
  (ADR-22).
- `trash` legt ein Objekt mit allem darunter in den Papierkorb, aus dem eine Person es binnen 30 Tagen zurückholt. Ein
  Agent löscht nichts endgültig.

## Dateien

Eine Datei erreicht einen Agenten als Text, Seite für Seite, nie als ihre Bytes: `read_file_page` liefert eine Seite des
Texts eines PDF oder den Text einer anderen Datei als eine Seite, den besten Text zuerst (was der Inhalt selbst sagt vor
dem, was aus Pixeln erkannt wurde).

Eine Datei erreicht UnderDoc über einen **Einmal-Upload-Link**, nie in einer Nachricht:

1. `upload_link` beginnt einen Upload mit den Rechten der Person, für einen Namen und eine genaue Größe, und liefert einen
   Link `https://<host>/s/u/<token>` mit einer `curl`-Zeile.
2. Ein Programm schickt die Datei dorthin als Körper eines einzigen `PUT`, binnen 15 Minuten. Seine Antwort nennt, was
   angelegt wurde.

Der Link wirkt einmal: Ihn zu nehmen entfernt ihn, auch wenn der Upload scheitert. Gespeichert wird nur der Hash seines
Tokens. Er liegt unter `/s/`, das das Gate offen lässt wie für Freigabelinks, denn das Programm, das die Datei schickt, ist
nicht angemeldet; allein das Token sagt, was es darf. Die Rechte werden geprüft, wenn der Link entsteht, und noch einmal,
wenn die Datei angelegt wird, als Agent der Person. Eine Anfrage eines Browsers einer anderen Website wird abgelehnt.

## Suche

Die Suche liefert Abschnitte (ADR-24): ein Datensatz je Überschrift und was folgt, ein Treffer je Objekt, Codes in
eigenen Feldern und exakt, die Rechte als Filter und jeder Treffer noch einmal in der Datenbank geprüft. Der Text eines
PDF hält seine Seiten auseinander, auf dem Server und im Browser, also ist jede Seite eigene Datensätze, und ein Treffer
nennt seine Seite (`#page=3`). Ein Wort verträgt ab vier Buchstaben einen Tippfehler (T-24, gemessen in
[search.md](../../docs/search.md)).

## Das Archiv

Archivierte Bereiche ([rights.md](../../docs/rights.md#archived-spaces), T-34) sind für alle nur lesbar, und `context`
und `search` lassen sie weg, bis der Agent mit `archived: true` danach fragt (der Baum führt sie dann unter den anderen,
markiert „archived, read only“; ohne die Angabe sagt `context`, wie viele er weggelassen hat). `read` und `used_in`
öffnen, was darin liegt, über seine Adresse. Jedes Werkzeug, das etwas ändert, und ein vorher geholter `upload_link`
werden dort abgelehnt („this space is archived“). Agenten archivieren und stellen nie einen Bereich wieder her.

## Was noch fehlt

- **Lücken des Browsers** (ein Werkzeug, das der Browser nicht kann, ist eine fehlende Funktion der Oberfläche,
  ADR-34): Die Beschreibung einer Seite lässt sich im Browser nicht setzen, also setzt `set_front_matter` sie auch
  nicht; ein Treffer in einem PDF (`#page=3`) öffnet die Datei am Anfang.
- Die Wege für etwas, das verwendet wird (überall löschen, eine Kopie je Verwendung, eine neue Quelle), wählt eine
  Person im Browser.
- Den Text eines Anhangs lesen, Verlauf und ältere Versionen, Ordner, Sammlungen und Gruppen sind keine Werkzeuge:
  Menschen tun das im Browser (ADR-34).
- Einen Block innerhalb einer Seite zu verschieben macht keine Version, also trägt es keinen KI-Hinweis (T-20).
- Andere Quellen (`source_search`, `source_read`) und die Werkzeuge von Konnektoren kommen mit T-31.
- Automatische Abschnitte (T-30) gibt es noch nicht, also hat `read` nichts zu zählen.
