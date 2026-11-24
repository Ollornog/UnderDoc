# Konten

Deutsche Übersetzung von [docs/accounts.md](../../docs/accounts.md); maßgeblich ist das englische Original.

Angemeldet wird über die Anmeldeschranke vor UnderDoc, TinySesam ([sign-in.md](../../docs/sign-in.md),
[ADR-32](../../backlog/ADR-32-sign-in-only-through-the-gate.md)). Die Schranke führt die Konten; UnderDoc hält keine
Zugangsdaten einer Person. Seit T-33 bittet UnderDoc die Schranke selbst über ihre **Konten-API**, eine Kollegin
einzuladen, einen neuen Anmeldelink zu geben, ein Konto zu sperren und zu entsperren und seinen Zugang zurückzusetzen, und
lässt endgültiges Löschen von den **Freigaben** der Schranke bestätigen
([ADR-35](../../backlog/ADR-35-what-the-gate-decides-and-what-the-app-decides.md)).

All das ist **optional**: Ohne die Einstellungen unten wird nichts angeboten, die Knöpfe fehlen, und eine Instanz hinter
einer Schranke ohne Konten-API für sie arbeitet wie bisher. Auch ohne sie gilt: die Sperre von UnderDoc (unten), der
delegierte Administrator und `underdoc account add`.

## Wer was darf

Die Schranke nimmt die Person, für die UnderDoc zu handeln angibt (`TinySesam-On-Behalf-Of`), auf Treu und Glauben; sie
prüft nur das Dienstkonto von UnderDoc, seine Scopes, seine Rollen-Allowlist und seine Grenzen. Deshalb **prüft UnderDoc
selbst**, wer was darf, bevor es die Schranke ruft:

| Wer | Lädt ein | Neuer Link, sperren, entsperren | Setzt Zugang zurück |
|---|---|---|---|
| **Administrator** (`UNDERDOC_ADMIN_ROLE`) | in jede lokale Gruppe | jedes Konto außer dem eigenen | jedes Konto, als Freigeber der Schranke |
| **Delegierter Administrator** (`UNDERDOC_DELEGATE_ROLE`) | in jede lokale Gruppe | jedes Konto außer dem eines Administrators, eines anderen Delegierten oder dem eigenen | nein |
| **Gruppen-Admin** | in die Gruppen, die er verwaltet | die Personen, die nur in seinen Gruppen sind oder die er eingeladen hat ([unten](#gruppen-admins)) | nein |
| Alle anderen | nein | nein | nein |

- **Nur eine Person im Browser**, angemeldet als Sitzung (`Remote-Auth: session`): nie ein Agent, ein Schlüssel, ein
  App-Passwort, OAuth, MCP oder ein Dienstkonto, gleich welche Rollen es trägt (ADR-36, B-4). MCP hat dafür kein Werkzeug.
- **Nie ein Dienstkonto** als betroffenes Konto, und nie das eigene Konto.
- **Eine Gruppe** aus der Schranke oder eine archivierte Gruppe nimmt keine Einladung.
- **Ob ein Konto einem Administrator oder Delegierten gehört**, sagen die Markierung der Kommandozeile, die Rollen seiner
  letzten Anmeldung und die Rollen, die die Schranke ihm jetzt gibt, wo sie UnderDoc das Konto sehen lässt (B-7): Ein
  Konto aus `add` oder `adopt` oder eines, das seit seiner letzten Anmeldung befördert wurde, zählt als das, was die
  Schranke sagt. Antwortet die Schranke nicht, handelt nur ein Administrator daran, außer beim Sperren, das auch ohne
  Schranke geht.
- **Rollen:** Eine Einladung gibt nur Rollen der Allowlist von UnderDoc (`UNDERDOC_ACCOUNTS_ROLES`), nie die Rolle eines
  Administrators, des delegierten Administrators, derer, die Bereiche anlegen, oder die Freigeber-Rolle der Schranke; der
  Start lehnt eine solche Liste ab. Die Schranke führt ihre eigene Allowlist und lehnt alles außerhalb ebenfalls ab.

<a id="gruppen-admins"></a>
### Gruppen-Admins (B-7)

Ein Gruppen-Admin kann jedes Konto in seine Gruppe aufnehmen; das allein gibt ihm also nichts über die Person. Er verwaltet:

- die Personen, die **nur in seinen Gruppen** sind (lokale Gruppen; Rollen und Gruppen aus der Schranke zählen nicht),
  und
- die Personen, **die er eingeladen hat**: UnderDoc merkt sich, wer ein Konto eingeladen hat (`accounts.invited_by`,
  Migration 042).

Davon bekommt einen **neuen Anmeldelink** nur, wen er eingeladen hat; Administrator und delegierter Administrator geben ihn
jedem, den sie verwalten. Und ein Gruppen-Admin **sperrt niemanden**: **Remove from my groups** (die Sperre eines
Gruppen-Admins) nimmt die Person aus jeder Gruppe, die er verwaltet, und sperrt weder das Konto in UnderDoc noch das in der
Schranke; zu entsperren gibt es nichts, er nimmt die Person wieder in die Gruppe auf. Zugang zurücksetzen bleibt bei den
Administratoren.

## Einladen

Ein Gruppen-Admin öffnet im Reiter **Rights** eine Gruppe, **Members**, und lädt eine Kollegin mit Login, Name und auf
Wunsch Mailadresse ein. UnderDoc bittet die Schranke, das Konto anzulegen (`POST /auth/api/users`, mit den Rollen von
UnderDoc), und die Schranke antwortet mit seiner dauerhaften ID und einem Einmal-Anmeldelink. UnderDoc **bindet sofort ein
Profil an diese ID** und nimmt es in die Gruppe auf, so ist alles, was mit der Gruppe geteilt ist, bei der ersten Anmeldung
da. Der Link wird **einmal** gezeigt, zum Übergeben; UnderDoc speichert und protokolliert ihn nirgends. Ein Profil wird nur
über die dauerhafte ID der Schranke gebunden: Eine Einladung unter dem Login von jemandem in UnderDoc legt ein anderes
Konto an und übernimmt nie dessen Konto.

## Neuer Anmeldelink, sperren, Zugang zurücksetzen

**Account** neben einer Person in einer Gruppe, und bei den Benutzern für Administratoren und Delegierte, zeigt das Konto
in UnderDoc und in der Schranke, mit dem, was der Betrachter darf:

- **New sign-in link:** nur vor der ersten Anmeldung und nicht, solange das Konto gesperrt ist; ältere Links verfallen.
  Nach der ersten Anmeldung wäre ein Link eine Übernahme, und die Schranke lehnt ihn ab.
- **Lock** und **Unlock** (Administratoren und Delegierte): in der Schranke, wo sie es UnderDoc erlaubt (sie verwaltet nur
  Konten, die UnderDoc angelegt hat oder übergeben bekam), und **immer in UnderDoc** (die Sperre von UnderDoc). Gesperrt
  wird erst in UnderDoc, dann in der Schranke; entsperrt erst in der Schranke, dann in UnderDoc, so bleibt das Konto bei
  einem Fehler unterwegs gesperrt (B-7). Ein Konto, das der Betreiber der Schranke gesperrt hat, entsperrt nur der
  Betreiber. Einem Gruppen-Admin wird stattdessen **Remove from my groups** angeboten ([oben](#gruppen-admins)).
- **Reset access** (neues Telefon, vergessenes Passwort), nach der ersten Anmeldung: UnderDoc bittet die Schranke um ihre
  Freigabe `tinysesam.account.reset_access`, und wer gefragt hat, **bestätigt sie in der Schranke** mit dem angezeigten
  Code und einem eigenen Step-up. Die Schranke lässt nur einen Inhaber ihrer Freigeber-Rolle bestätigen, die ADR-35 auf die
  Administratorrolle von UnderDoc setzt; deshalb wird es nur Administratoren angeboten, und diese Rolle steht nie in der
  Allowlist. Danach sind Passwort, Faktoren, Schlüssel und Sitzungen des Kontos weg, und UnderDoc holt den neuen
  Anmeldelink **einmal** ab und zeigt ihn dem, der gefragt hat.

## Die Sperre von UnderDoc

Ein gesperrtes Konto kommt über **keinen Weg** herein: Arbeitsbereich, API, MCP und seine Upload-Links antworten *Locked*
(403), sein Samba-Benutzer wird von der Freigabe genommen, und die eine Prüfung der Rechte verweigert ihm jede Aktion
(`locked` als entscheidender Eintrag), also geben weder eigene Objekte noch Einträge noch Links, die es angelegt oder
genommen hat, etwas her: Ein Freigabelink von ihm antwortet, solange es gesperrt ist, wie einer, den es nie gab. Nichts
geht verloren; Entsperren bringt alles zurück. Die Sperre gilt für sich, auch wo die Schranke nicht sperrt, und bis ein
Schranken-Token des Kontos abläuft. In der Datenbank: `accounts.locked`, `locked_by` (Migration 040).

Grenzen (B-7): Die Rechte gelten sofort, aber eine andere Instanz von UnderDoc vor derselben Datenbank behält an ihrer Tür
bis zu **5 Minuten**, was sie über ein Konto weiß; ein gesperrtes Konto kann dort so lange noch Seiten öffnen (die eine
Prüfung verweigert ihm trotzdem jede Aktion), und eine **Samba-Sitzung**, die vor der Sperre offen war, bleibt offen, bis
die Freigabe den Benutzer abnimmt.

## Zwei Stufen der Verwaltung

- Der **Administrator** trägt die Rolle der App `UNDERDOC_ADMIN_ROLE` (`underdoc-admin`) oder die Marke der
  Kommandozeile. Er darf alles unten und mehr.
- Der **delegierte Administrator** trägt die eigene Rolle der App `UNDERDOC_DELEGATE_ROLE` (`underdoc-delegate`), weder
  die des Administrators noch die zum Anlegen von Bereichen (der Start lehnt beides ab, B-7):
  - verwaltet die Konten aller Gruppen: lädt in jede lokale Gruppe ein, gibt neue Anmeldelinks, sperrt und entsperrt;
  - sieht die Benutzer und die Mitglieder jeder Gruppe;
  - archiviert Gruppen und stellt sie wieder her (B-5).

  Nicht: Module, wie lange das Protokoll Einträge behält, die Regelseite, die Marken der Administratoren, Rechte an
  Objekten über die eigenen hinaus, die Konten von Administratoren und Delegierten, Zugang zurücksetzen.
- Ein Dienstkonto ist keins von beiden, gleich welche Rollen ihm die Schranke gibt, und ein Agent von beiden hat nichts
  davon (B-4).

Die Rechte werden an einer Stelle entschieden (`account.User.Has` mit `PermAccounts`, `PermArchiveGroups`,
`PermAdminister`), damit sie später in die eigenen Rechte der Schranke umziehen können (die geplante Berechtigungsschicht
von TinySesam), ohne dass sich der Rest von UnderDoc ändert.

## Endgültig löschen mit Freigabe

Mit `UNDERDOC_APPROVALS=on` brauchen endgültiges Löschen aus dem Papierkorb und das Leeren des Papierkorbs zusätzlich eine
**signierte Einmal-Freigabe der Schranke** (B-3): Der Go-Client der Schranke prüft sie (`VerifyApproval`), ihre ID
(`jti`) gilt einmal (Tabelle `approval_jti`).

1. Im Papierkorb **Delete for good** mit eingetipptem Namen (oder **Empty the trash** mit `Trash`). UnderDoc prüft zuerst
   alles: Besitzer oder Administrator, Browser-Sitzung, der Name, alles darunter darf weg.
2. Erst dann fragt UnderDoc die Schranke (`<app>.object.purge` mit Objekt und Titel, oder `<app>.trash.empty` mit der Zahl
   der Einträge und einem **Stichtag**) und schickt den Browser auf die Seite der Schranke. Dort vergleicht die Person den
   Code und bestätigt mit einem Step-up.
3. Die Schranke schickt den Browser zurück an `/approvals/return`. Zurück kommt nur der Browser, der gefragt hat (ein
   Cookie mit dem Zustand, verglichen mit dem, den die Schranke zurückgibt, und dem, den UnderDoc behalten hat), nur das
   Konto, das gefragt hat, einmal.
4. UnderDoc holt die Bestätigung ab, sperrt, was gelöscht wird, **prüft erneut**, prüft dann die Bestätigung (diese
   Aktion, diese Felder, diese Person, dieser Browser, einmal) und löscht. Ein Löschen, das die Prüfung auf dem Rückweg
   ablehnt, verbraucht keine Bestätigung; das Leeren nimmt nur, was bis zum Stichtag gelöscht wurde, nie, was danach kam
   (B-6).

Ohne die Einstellung bleibt endgültiges Löschen wie bisher: Besitzer oder Administrator, im Browser, mit eingetipptem
Namen.

## Konten, die es schon gibt

Die Konten-API der Schranke zeigt UnderDoc nur die Konten, die es selbst angelegt hat. Bestehende Konten übergibt der
**Betreiber der Schranke** an UnderDoc (`tinysesam accounts adopt --app underdoc …` oder *Take over for …* im Panel der
Schranke). Danach:

```bash
underdoc account [--tenant <id>] adopt [--dry-run]                # ein Konto je Konto, das die Schranke UnderDoc zeigt
underdoc account [--tenant <id>] add <remote-id> <login> <name>   # ein Konto, gebunden an eine dauerhafte ID, für einen Import
underdoc account [--tenant <id>] list                             # alle Konten mit ID, Herkunft und Marke
```

Beide binden nur über die dauerhafte ID und geben keine Administratormarke; eine schon gebundene ID behält ihr Konto.

## Einstellungen

| Einstellung | Bedeutung | Vorgabe |
|---|---|---|
| `UNDERDOC_GATE_URL` | Die eigene Adresse der Schranke, ihre `base_url`: Die Aufrufe gehen direkt dorthin, nicht über den Proxy von UnderDoc. https, reines http nur zur eigenen Maschine | keine |
| `UNDERDOC_GATE_SERVICE_KEY_FILE` | Eine Datei, die den Automationsschlüssel des Dienstkontos von UnderDoc in der Schranke enthält und sonst nichts (Modus 0600). Nie der Schlüssel selbst in der Umgebung und nie in einem Log | keine |
| `UNDERDOC_GATE_APP` | Die ID von UnderDoc in den `apps` der Schranke | `underdoc` |
| `UNDERDOC_ACCOUNTS_ROLES` | Die Rollen, die UnderDoc einer eingeladenen Person gibt, durch Kommas getrennt, ein Teil der Allowlist der Schranke. Gesetzt, schaltet sie die Konten ein | keine: aus |
| `UNDERDOC_APPROVALS` | `on`: endgültiges Löschen wartet auf eine Freigabe der Schranke | aus |
| `UNDERDOC_DELEGATE_ROLE` | Die Rolle der App, die einen delegierten Administrator macht | `underdoc-delegate` |

**Beim Start** fragt UnderDoc die Schranke, was sein Schlüssel darf (`GET /auth/api/provisioning`), und startet nicht, wenn
die App eine andere ist, ein Scope fehlt (`users:read`, `users:invite`, `users:disable`; `users:reset` schaltet das
Zurücksetzen ein), eine Rolle aus `UNDERDOC_ACCOUNTS_ROLES` nicht auf der Allowlist der Schranke steht oder eine davon die
Freigeber-Rolle oder eine Rolle von Administratoren ist. Konten und Freigaben brauchen die Datenbank.

**In der Schranke** richtet der Betreiber die App so ein, wie es die Doku der Schranke sagt; ein Beispiel steht im
englischen Original. UnderDoc selbst spricht mit der Schranke nur HTTP und nutzt ihren Go-Client, nie die Python-API von TinySesam. Der Test
dieses Repos fährt genau eine solche Konfiguration gegen ein echtes TinySesam
([`scripts/gate/gate.py`](../../scripts/gate/gate.py), in `scripts/check-services.sh`). Bis das fertige Gateway der
Schranke `apps` aus seiner eigenen Konfiguration liest (TinySesam T-97, eine Konfigurationsdatei in TOML), ist eine
Schranke mit Konten-API TinySesam als Bibliothek mit einer solchen Konfiguration. Später soll TinySesam als Begleiter von
UnderDoc in dessen Compose-Datei laufen, mit lokalen Konten und OIDC nebeneinander; die HTTP-Aufrufe bleiben dieselben.

## Das Protokoll

Jeder dieser Vorgänge ist eine Zeile im Protokoll (ADR-11), mit der dauerhaften ID, die UnderDoc der Schranke als die
Person nannte, für die es handelte: `invited`, `sign-in link`, `locked`, `unlocked`, `access reset`, `account added`
(Kommandozeile und adopt) und `deleted for good` mit *confirmed in the gate*. Ein Anmeldelink steht nie darin.
