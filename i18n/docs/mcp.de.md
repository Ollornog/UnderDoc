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
| `context` | Hier anfangen: für wen der Agent handelt und was er darf, die Regelseite, offene Hinweise, der Baum bis zu einer Tiefe (2, wenn nicht anders verlangt, höchstens 4, höchstens 200 Einträge; `under` für einen Teil davon) |
| `read` | Eine bis fünf Seiten als Markdown mit Front Matter, der Markierung vor jedem Block und der Version, gegen die geschrieben wird; oder ein Block oder Abschnitt (`#b3`); eine Seite der Ordnerquelle als Gliederung |
| `used_in` | Die Seiten, die eine Seite, einen Block, Abschnitt oder Teil einbetten |
| `search` | Eine Suche über die Abschnitte von Seiten und die Seiten von PDF und anderen Dateien: eine Zeile je Treffer mit Adresse, Abschnitt und Auszug, höchstens 20 Treffer, die Gesamtzahl am Ende; Fehlercodes exakt |

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

## Suche

Die Suche liefert Abschnitte (ADR-24): ein Datensatz je Überschrift und was folgt, ein Treffer je Objekt, Codes in
eigenen Feldern und exakt, die Rechte als Filter und jeder Treffer noch einmal in der Datenbank geprüft. Der Text eines
PDF hält seine Seiten auseinander, auf dem Server und im Browser, also ist jede Seite eigene Datensätze, und ein Treffer
nennt seine Seite (`#page=3`). Ein Wort verträgt ab vier Buchstaben einen Tippfehler (T-24, gemessen in
[search.md](../../docs/search.md)).

## Was noch fehlt

- Schreiben, Verschieben, Papierkorb und Dateien kommen im zweiten Teil von T-21.
- Andere Quellen (`source_search`, `source_read`) und die Werkzeuge von Konnektoren kommen mit T-31.
- Automatische Abschnitte (T-30) gibt es noch nicht, also hat `read` nichts zu zählen.
