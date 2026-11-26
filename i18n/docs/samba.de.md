# Samba

Deutsche Übersetzung von [docs/samba.md](../../docs/samba.md); maßgeblich ist das englische Original.

Dateien der Ansicht **Dateien** als Netzlaufwerk unter Windows, macOS und Linux, über SMB
([ADR-30](../../backlog/ADR-30-samba-share.md), ADR-28, ADR-10). Was dort gespeichert wird, wird eine Version in UnderDoc,
und was sich im Web ändert, erscheint auf der Freigabe. UnderDoc läuft nie als root: Ein **Begleiter**-Container führt
smbd aus, und UnderDoc steuert ihn über einen Unix-Socket.

## Was freigegeben wird

- **Company** (`\\<server>\Company`): der Ordner des Mandanten, sobald der Mandant einen Begleiter hat.
- **Meine Dateien** (`\\<server>\<benutzername>`): der Ordner jedes Kontos mit App-Passwort (mit Domäne: das SMB
  eingeschaltet hat), über `[homes]`, nur für dieses Konto.
- **Sammlungen**, deren Verwalter in ihrem Rechtedialog **Über Samba freigeben** eingeschaltet haben, je eine Freigabe
  mit ihrem Namen.
- **Ein Ordner, in dem jemand eine Rolle hält, ohne den Ordner darüber zu lesen**, ist für diese Person eine eigene
  Freigabe mit dem Namen des Ordners; sie erreicht ihn, ohne zu sehen, was darüber liegt (ADR-29).

Nur freigegebene Bibliotheken liegen auf der Platte; der Inhaltsspeicher behält so oder so jede Version. Eine Bibliothek,
die nicht mehr freigegeben ist, verlässt die Platte, nachdem ein letzter Durchgang übernommen hat, was sich dort
geändert hat; wird dort etwas nicht übernommen (Schreiber unbekannt oder abgelehnt), bleibt das Verzeichnis liegen, das
Protokoll sagt was, und jeder Durchgang schaut erneut.

## Rechte auf der Freigabe

Die Aktionen jedes Kontos (ADR-36) werden POSIX-ACLs je Ordner: Lesen, wo es lesen darf, Schreiben, wo es lesen,
anlegen, schreiben und löschen darf, nichts, wo es nicht lesen darf. Jedes Konto wird für sich entschieden, eine
Verweigerung auf einem Ordner gilt also auf der Freigabe wie im Web. `hide unreadable` verbirgt, was jemand nicht lesen
darf, und `access based share enum` listet nur die Freigaben, die jemand erreicht. **Samba kennt Rechte nur je Ordner**
([T-1](../../backlog/T-1-measure-office-saving.md)): Ein Speichern über eine temporäre Datei und ein Umbenennen erzeugt
eine neue Datei mit den Rechten ihres Ordners, ein Recht auf einer einzelnen Datei überlebte das nicht. In einer
freigegebenen Bibliothek gilt eine Erlaubnis auf einer einzelnen Datei nur im Web, und der Rechtedialog sagt das; eine
Verweigerung auf einer einzelnen Datei und das Brechen ihrer Vererbung werden dort abgelehnt (die Freigabe zeigte, was
das Web verbirgt), ein Ordner kann beides. Aus demselben Grund wird eine Sammlung mit einer Datei, die die Vererbung
bricht oder jemandem etwas verweigert, nicht freigegeben, und nichts mit einer solchen Datei wird in eine freigegebene
Bibliothek verschoben: Beides antwortet `409` mit den Dateien, die zuerst wieder erben und ihre Verweigerung verlieren
müssen. Eine Datei, die trotzdem in einer freigegebenen Bibliothek liegt (sie brach die Vererbung, bevor der Mandant
einen Begleiter hatte, oder kam aus dem Papierkorb zurück), bleibt von der Platte fern, und das Protokoll sagt es.

Eine **archivierte** Bibliothek ([rights.md](../../docs/rights.md#archived-spaces), T-34) ist auf der Freigabe für alle nur
lesbar, bis sie wiederhergestellt wird: Die eine Prüfung lehnt dort Anlegen, Schreiben und Löschen ab, die ACLs geben
also nur Lesen, und eine neue Datei oder Version über SMB wird abgelehnt wie eine ohne Rechte.

Eine Datei, die in einen anderen Ordner verschoben wurde, im Web oder über SMB, und eine über ein Umbenennen gespeicherte
Datei bekommen beim nächsten Durchgang die ACL ihres Ordners und gehören ab dann der UID von UnderDoc.

## Das App-Passwort

Im Profil erzeugt **SMB-Freigabe** ein App-Passwort: Der Benutzername ist der Login, wenn er passt (Kleinbuchstaben,
Ziffern, `.`, `_`, `-`, bis 32 Zeichen), sonst ein dort gewählter, und er bleibt. Das Passwort (20 Zeichen in
Fünfergruppen) wird einmal gezeigt. Ein Name, der der Name einer Freigabe ist (`company` neben `\\files\Company`, in
jeder Schreibweise), wird abgelehnt: Sein `[homes]` würde die Freigabe verdrängen. Eine Freigabe behält ihren Namen
trotzdem, sollte es einen solchen Benutzer geben. UnderDoc behält nur seinen NT-Hash, versiegelt mit
`UNDERDOC_SECRET_KEY`; Samba braucht also einen Tresorschlüssel. Ein neues App-Passwort ersetzt das alte und beendet
dessen Sitzungen; Entfernen beendet den Samba-Benutzer. Administratoren listen die Samba-Benutzer und entfernen jeden.

```text
GET    /api/samba                       an, Server, der Samba-Benutzer des Betrachters und die Freigaben, die er erreicht
POST   /api/samba/password              {"name": "anna"}  → {"name", "password", "server"}, einmal gezeigt
POST   /api/samba/domain                mit Domäne: der Samba-Benutzer des eigenen Domänenkontos → {"name", "server"}
DELETE /api/samba/password              den eigenen entfernen (auch mit Domäne)
GET    /api/samba/accounts              Administratoren: jeder Samba-Benutzer
DELETE /api/samba/accounts/<konto>      Administratoren
PUT    /api/files/roots/<id>/samba      {"on": true}   Verwalter einer Sammlung der Dateien
```

Die Faktenleiste einer Datei zeigt ihren Pfad auf der Freigabe (`\\files\Company\Verträge\plan.xlsx`) und wer sie dort
offen hat; `GET /api/files/<id>` antwortet damit als `share`.

## Eine Domäne (optional)

Statt App-Passwörtern kann der Begleiter eines Mandanten Mitglied einer Active-Directory-Domäne sein (T-42, ADR-30): Man
meldet sich mit dem Domänenkonto an der Freigabe an, per Kerberos von einem Rechner der Domäne oder per NTLMv2 mit
dessen Passwort. Ohne die Einstellungen unten bleibt alles wie oben.

- **Ein Modus je Mandant, beim Einrichten gewählt, nie gemischt** (Entscheidung des Maintainers vom 2026-11-24). Mit
  Domäne erreichen nur Domänenkonten die Freigabe: Es gibt keine App-Passwörter, der Begleiter hält überhaupt kein
  lokales Passwort, und eine Anmeldung als lokaler Benutzer des Begleiters (`FILES\anna`) scheitert. Der Begleiter ordnet
  ein Domänenkonto über den Namen einer UID zu; ein lokaler Benutzer gleichen Namens wäre für den Kernel dieselbe Person.
  UnderDoc startet nicht, solange ein Mandant Samba-Benutzer des anderen Modus hält, und der Begleiter lehnt einen
  Zustand des anderen Modus ab; den Modus zu wechseln ist eine Migration (unten).
- **Wer die Freigabe erreicht.** Im Profil bietet **SMB-Freigabe** **Mein Domänenkonto verwenden** an: Das Konto bekommt
  einen Samba-Benutzer mit dem Namen seines Domänenkontos und eine UID aus dem Bereich, ohne Passwort. Der Name kommt aus
  der Einrichtung, nie vom Konto: sein Login in Kleinbuchstaben oder die bleibende ID, die die Schranke ihm gibt
  (`UNDERDOC_SAMBA_DOMAIN_NAMES`). Gewählt wird, was zu den Namen der Domäne passt; der Login passt, wo die Schranke ihre
  Konten aus demselben Verzeichnis nimmt. SMB zu beenden entfernt den Samba-Benutzer, wie das Entfernen eines
  App-Passworts.
- **Was der Begleiter einlässt.** winbindd ordnet ein Domänenkonto der UID des gleichnamigen Samba-Benutzers zu
  (`idmap_nss` für die Domäne, innerhalb von `UNDERDOC_SAMBA_UIDS`); das Abbild hat kein `libnss-winbind`, die Benutzer
  des Systems sind also nur die, die UnderDoc übergeben hat. Ein Domänenkonto ohne Samba-Benutzer (nie eingeschaltet oder
  entfernt), ein Benutzer des Systems (`root`, `daemon`, …, deren UIDs außerhalb des Bereichs liegen) und der
  Administrator der Domäne bekommen keine UID und werden mit dem richtigen Passwort abgewiesen. Großschreibung
  (`EXAMPLE\DORA`) ist dasselbe Konto. Wann immer sich die Benutzer ändern, leert der Begleiter die Caches von winbindd:
  Eine UID, die an ein anderes Konto übergeht, erreicht das Domänenkonto, das sie vorher hatte, nie, und ein gerade
  eingeschaltetes Konto wird sofort eingelassen.
- **Rechte, Protokoll und alles Übrige** sind wie mit App-Passwörtern: Der Samba-Benutzer ist das Konto, und sein Name
  ist, was das Protokoll von smbd und die offenen Dateien nennen.

| Einstellung von UnderDoc | Bedeutung | Vorgabe |
|---|---|---|
| `UNDERDOC_SAMBA_DOMAIN` | Mandanten aus `UNDERDOC_SAMBA`, deren Begleiter Mitglied einer Domäne ist, durch Kommas getrennt | keine: App-Passwörter |
| `UNDERDOC_SAMBA_DOMAIN_NAMES` | Was das Domänenkonto eines Kontos benennt: `login` oder `remote-id` | `login` |

| Einstellung des Begleiters | Bedeutung | Vorgabe |
|---|---|---|
| `UNDERDOC_SAMBA_DOMAIN_REALM` | Der Realm der Domäne, etwa `EXAMPLE.TEST`; macht den Begleiter zum Mitglied | keine: App-Passwörter |
| `UNDERDOC_SAMBA_DOMAIN_WORKGROUP` | Der Kurzname der Domäne, etwa `EXAMPLE` | Pflicht mit Realm |
| `UNDERDOC_SAMBA_DOMAIN_JOIN_USER` | Ein Konto, das Rechner in die Domäne aufnehmen darf; nur nötig, solange nicht beigetreten | keines |
| `UNDERDOC_SAMBA_DOMAIN_JOIN_PASSWORD_FILE` | Eine Datei mit dessen Passwort (ein Geheimnis des Containers, nur lesbar eingebunden) | keine |
| `UNDERDOC_SAMBA_DOMAIN_STATE` | Ein Volume für das Geheimnis des Maschinenkontos (`secrets.tdb`) | `/var/lib/underdoc-samba-domain` |

- **Der Beitritt.** Beim Start prüft der Begleiter seine Mitgliedschaft (`net ads testjoin`). Ist er kein Mitglied, tritt
  er mit dem Konto und der Passwortdatei bei (`net ads join`; Samba liest das Passwort aus der Datei, nie von einer
  Befehlszeile oder aus der Umgebung; ein Passwort in `UNDERDOC_SAMBA_DOMAIN_JOIN_PASSWORD` wird abgelehnt), prüft
  erneut und startet erst dann winbindd und smbd. Kein Mitglied und keine Passwortdatei: Er endet und sagt, warum. Das
  Geheimnis des Maschinenkontos bleibt auf dem Volume, ein neuer Container ist also weiter Mitglied und braucht die
  Passwortdatei nicht mehr; nach dem Beitritt entfernen. Der Beitritt lässt das DNS der Domäne in Ruhe
  (`--no-dns-updates`): Dem Namen, den man tippt (`UNDERDOC_SAMBA_NAME`, in der Zone der Domäne), einen Adresseintrag auf
  die Adresse geben, die Port 445 veröffentlicht, damit Kerberos ihn findet.
- **Der Container** braucht die Domänencontroller als DNS-Server (`dns:` in compose) und erreicht sie auf den Ports von
  Kerberos, LDAP und SMB. Seine übrigen Rechte bleiben wie oben; er braucht keine weitere Fähigkeit.
- **Den Modus zu wechseln** ist eine Migration durch den Betreiber: UnderDoc anhalten, jeden Samba-Benutzer des
  Mandanten entfernen (`underdoc samba-users --tenant <id> remove`, das Protokoll hält es fest), `UNDERDOC_SAMBA_DOMAIN`
  und den Begleiter auf den anderen Modus stellen, beide starten. Danach schalten die Leute SMB in ihrem Profil wieder
  ein.

## Die Arbeitskopie

Jede Bibliothek liegt unter `<UNDERDOC_SHARE_DIR>/<mandant>/<ID ihrer Wurzel>/…` mit den echten Namen der Ordner und
Dateien, und jede Datei und jeder Ordner trägt seine ID im erweiterten Attribut `user.underdoc.id`. UnderDoc beobachtet
den Baum mit inotify und übernimmt eine Änderung zwei Sekunden nach dem letzten Ereignis, sobald die Datei nicht mehr
zum Schreiben offen ist:

- eine geänderte Datei wird eine Version dessen, der sie schrieb (aus dem Protokoll von smbd, sonst der Samba-Benutzer,
  dem die neue Datei auf der Platte gehört);
- eine neue Datei oder ein neuer Ordner wird ein Objekt dessen, der sie anlegte;
- ein Umbenennen oder Verschieben benennt das Objekt um oder verschiebt es;
- eine Datei ohne ID, wo eine Datei war, ist eine neue Version davon, so speichern Office und LibreOffice (T-1);
- eine gelöschte Datei oder ein gelöschter Ordner geht in den Papierkorb;
- temporäre Dateien (`~$*`, `*.tmp`, `.~lock.*#`, `Thumbs.db`, `desktop.ini`, `.DS_Store`, `._*`) werden nie Objekte.
  Diese Muster gelten nur für Dateien: Ein Ordner namens `Backup.tmp` ist ein gewöhnlicher Ordner. Eine im Web angelegte
  Datei mit einem solchen Namen bleibt von der Platte fern;
- ein harter Link (SMB2 legt einen an) endet beim nächsten Durchgang: Der Name, den UnderDoc kennt, behält das Objekt und
  seine ID, jeder andere Name wird eine eigene Kopie, eine neue Datei dessen, der den Link anlegte. Samba hat keine
  Einstellung gegen harte Links.

**Ein Schreiber, den niemand nennt, ist niemand:** Nennen weder das Protokoll von smbd noch der Eigentümer der Datei auf
der Platte, wer etwas geändert hat, wird die Änderung nie als die des Eigentümers des Objekts oder der Bibliothek
übernommen. Verschieben, Umbenennen, Löschen und Speichern werden aus der Datenbank zurückgesetzt; Neues bleibt auf der
Platte, ohne ein Objekt zu werden; das Protokoll nennt jedes (Aktion `samba`). Das Protokoll von smbd behält nur
Vorgänge, die etwas ändern (ein Auflisten oder Lesen verdrängt sie nicht), nummeriert sie, und ein Durchgang, der seit
dem vorigen fehlende Ereignisse findet, sagt das im Protokoll.

Jede Änderung geschieht als der Benutzer, der sie machte, durch dieselben Prüfungen wie im Web. Was die Datenbank ablehnt,
wird auf der Platte zurückgesetzt (Umbenennen, Verschieben, Löschen) oder bleibt dort ohne Version (neuer Inhalt, etwa
über dem Kontingent). Eine Änderung im Web schreibt über eine temporäre Datei und ein Umbenennen auf die Platte.
**Solange eine Datei über SMB offen ist, ändert das Web nichts an ihr:** Neue Version, Wiederherstellen, Umbenennen,
Verschieben oder Löschen antworten `409` mit dem, der sie offen hat. Ein voller Durchgang läuft beim Start, nach jeder
Änderung, die die Datenbank meldet, und stündlich; die Tabelle `share_files` hält fest, worauf sich Platte und Datenbank
zuletzt geeinigt haben, damit ein Neustart weiß, welche Seite sich geändert hat.

**Das Kontingent erreicht den Client** (2026-11-20, ADR-30). Nach jedem Durchgang gibt UnderDoc dem Begleiter den Platz
jeder freigegebenen Bibliothek, die eine Grenze bindet: das Kontingent der Bibliothek oder des eigenen Ordners eines
Benutzers, die Grenze des Mandanten (`UNDERDOC_FILES_QUOTA`) oder den Speicher, den der Hoster erlaubt, je nachdem, was am
wenigsten lässt, Versionen mitgezählt. smbd fragt ihn über `dfree command` ab (ein Link `dfree` auf den Helfer in dessen
Verzeichnis), Windows zeigt also diesen Platz am Laufwerk, und der Explorer lehnt eine Kopie, die nicht passt, vor dem
Start ab; eine Bibliothek ohne Grenze zeigt die Platte, und nichts zeigt mehr, als die Platte hat. smbd lehnt selbst kein
Schreiben ab: Ein Speichern, das nicht vorher nach Platz fragt, landet trotzdem auf der Platte und bleibt dort ohne
Version, ebenso eine Datei größer als `UNDERDOC_FILES_MAX_SIZE`; nichts schreibt den Inhalt der Datenbank darüber. **Wer
gespeichert hat, erfährt es:** einmal im Protokoll als seines („… über Samba wurde nicht übernommen: das Kontingent ist
voll“), beim Öffnen des Arbeitsbereichs (ein Hinweis) und im Profil unter SMB-Freigabe, bis Platz ist und die Datei
erneut gespeichert wird oder sie weg ist.

## Einrichtung

UnderDoc und der Begleiter teilen zwei Verzeichnisse unter denselben Pfaden: das Freigabeverzeichnis auf einem
Dateisystem mit erweiterten Attributen und POSIX-ACLs (ext4, XFS, btrfs; nicht NFS) und ein Verzeichnis für den Socket.
Bei vielen Mandanten bekommt jeder Mandant, der SMB will, einen eigenen Begleiter auf seinem eigenen Verzeichnis und
seiner eigenen Adresse; WebDAV ist die Tür für den Rest (M-7).

| Einstellung von UnderDoc | Bedeutung | Vorgabe |
|---|---|---|
| `UNDERDOC_SAMBA` | Mandanten mit Begleiter und ihre Sockets: `main=/run/underdoc-samba/helper.sock`, durch Kommas getrennt | keine: kein Samba |
| `UNDERDOC_SHARE_DIR` | Das Freigabeverzeichnis, derselbe Pfad in jedem Begleiter | keine |
| `UNDERDOC_SAMBA_SERVER` | Der Name des Servers, wie man ihn tippt, für Freigabepfade | keiner |
| `UNDERDOC_SAMBA_UIDS` | Die UIDs der Samba-Benutzer, innerhalb eines unprivilegierten Containers | `20000-29999` |

Samba braucht die Datenbank, den Inhaltsspeicher (`UNDERDOC_STORE_DIR`) und den Tresor (`UNDERDOC_SECRET_KEY`). Seiten der
Dokumente liegen nicht auf der Freigabe, ihre Passwörter also auch nicht ([secrets.md](../../docs/secrets.md)).

### Der Begleiter

Das Abbild wird aus [`deploy/samba/Dockerfile`](../../deploy/samba/Dockerfile) gebaut: smbd von Samba 4.24 aus den
Debian-13-Backports, der neuesten Reihe, die eine stabile Distribution führt, winbindd für eine Domäne und der Helfer
`underdoc-samba`. Ein Beispiel ist [`deploy/samba/compose.yml`](../../deploy/samba/compose.yml).

| Einstellung des Begleiters | Bedeutung | Vorgabe |
|---|---|---|
| `UNDERDOC_UID`, `UNDERDOC_GID` | UID und GID, unter denen UnderDoc läuft: das einzige Gegenüber des Sockets und Eigentümer jedes Pfads | `65532` |
| `UNDERDOC_SAMBA_ROOT` | Das Verzeichnis seines Mandanten, `<UNDERDOC_SHARE_DIR>/<mandant>` | Pflicht |
| `UNDERDOC_SAMBA_SOCKET` | Der Socket | `/run/underdoc-samba/helper.sock` |
| `UNDERDOC_SAMBA_NAME` | NetBIOS-Name und Serverbezeichnung | `UNDERDOC` |
| `UNDERDOC_SAMBA_UIDS` | Der Bereich der UIDs, derselbe wie bei UnderDoc | `20000-29999` |
| `UNDERDOC_SAMBA_AUDIT` | `all` protokolliert jeden Vorgang von smbd, für Aufnahmen wie T-1 | die ändernden |

- **Volumes:** nur `<UNDERDOC_SHARE_DIR>/<mandant>` unter demselben Pfad und das Verzeichnis des Sockets. Sein eigener
  Zustand (Benutzer, Passwörter, Freigaben, Datenbanken von smbd) liegt in `/var/lib/underdoc-samba`, einem `tmpfs`: Ein
  neuer Begleiter bekommt beim nächsten Durchgang alles von UnderDoc. Mit Domäne kommt das Volume des Maschinenkontos
  dazu (oben).
- **Rechte:** Er startet als root in seinem Container, weil smbd zur UID jedes Benutzers wechselt, mit nur `CHOWN`,
  `DAC_OVERRIDE`, `FOWNER`, `SETUID`, `SETGID` und `KILL`, `no-new-privileges` und einem nur lesbaren Wurzeldateisystem.
- **Port 445** nur auf einer Adresse des LAN oder des Mesh, nie im Internet. SMB 2.02 und neuer, nur NTLMv2, kein Gast,
  kein SMB1, keine POSIX-Erweiterungen, keine erweiterten Attribute von Clients, Signieren, wo der Client es anbietet.
- **Healthcheck:** `underdoc-samba probe` fragt den Helfer, der nur antwortet, solange smbd läuft.

### Der Kanal

Der Helfer legt den Socket mit Modus 0600 für `UNDERDOC_UID` an und prüft das Gegenüber jeder Verbindung
(`SO_PEERCRED`); jeder andere wird abgewiesen. Eine Verbindung trägt eine JSON-Anfrage aus einer geschlossenen Liste,
geprüft, bevor irgendetwas geschieht:

| Anfrage | Bewirkt |
|---|---|
| `ping` | antwortet, solange smbd (mit Domäne auch winbindd) läuft, mit den Versionen, seinem Modus (`local` oder `domain`) und einem Namen dieses Starts: Ein neu gestarteter Begleiter bekommt beim nächsten Durchgang seinen Zustand, einer des anderen Modus bekommt nichts |
| `apply` | der ganze Zustand: sein Modus, Benutzer (Name, UID, NT-Hash, Heimat; mit Domäne kein Hash) und Freigaben (Name, Pfad, Benutzer); schreibt sie, lädt smbd neu, beendet die Sitzungen von Benutzern, die weg sind oder deren Passwort sich änderte, und leert mit Domäne die Caches von winbindd, wenn sich die Benutzer geändert haben. Ein Zustand des anderen Modus wird abgelehnt. Derselbe Zustand zweimal ändert nichts |
| `acl` | setzt die ACL von Verzeichnissen, ihre Vorgabe-ACL und die der Dateien direkt darin und gibt jedes `UNDERDOC_UID`; jeder Pfad wird unterhalb des Mandantenverzeichnisses geöffnet, ohne Links zu folgen |
| `open` | die über SMB offenen Dateien, nach Benutzer, und ob zum Schreiben |
| `who` | die letzten Vorgänge im Protokoll von smbd, die etwas ändern, das der Helfer als Syslog seines Containers empfängt, nummeriert, mit den Nummern des ältesten und des neuesten, die er behält |
| `room` | der Platz jeder Bibliothek, die eine Grenze bindet (Pfad, frei, gesamt); der Helfer hält je Bibliothek eine Datei in `<run>/room` (durchschreitbar, nicht auflistbar) für `dfree command` und entfernt die nicht genannten. Ein Begleiter von vorher antwortet darauf mit „unbekannt“: UnderDoc sagt es einmal, und Clients sehen die Platte |

Keine Anfrage trägt eine Befehlszeile, und jeder Name, jede UID, jeder Hash und jeder Pfad wird gegen die Regeln des
Kanals geprüft (`internal/samba`). Ein Freigabepfad enthält nie `%`, das smb.conf als Variable wie `%U` läse: Ein Ordner
mit `%` im Pfad ist keine eigene Freigabe und bleibt über seine Bibliothek erreichbar; `%` in einem Kommentar wird ein
Leerzeichen.

## Verbinden von Windows

Im Explorer **Netzlaufwerk verbinden**, Ordner `\\<server>\Company` (oder die Freigabe, die das Profil nennt),
**Verbindung mit anderen Anmeldeinformationen herstellen**, Benutzername `<server>\<benutzername>` und das App-Passwort.
Windows behält es in seiner Anmeldeinformationsverwaltung. Ohne Domäne meldet sich Windows mit NTLMv2 an, das Microsoft
als veraltet führt, das aber weiter funktioniert. Mit Domäne meldet sich ein Rechner der Domäne von selbst per Kerberos
an; anderswo ist der Benutzername `<DOMÄNE>\<benutzername>` mit dem Passwort der Domäne.

## Grenzen

- ~~**Geratene Namen über einer eigenen Freigabe.**~~ Geschlossen am 2026-11-24 (T-43): Die Ordner, die jemand auf dem Weg
  zu einer eigenen Freigabe nur durchschreitet (`--x`), sind in der Freigabe der Bibliothek vor ihm verborgen
  (`veto files`, eine Liste je Benutzer und Freigabe, `include = …/%U.conf` in der Freigabe; bis die Datei eines Benutzers
  gelesen ist, verbirgt die Freigabe jeden solchen Ordner von irgendwem, eine fehlende Datei verbirgt also mehr, nie
  weniger). Ein geratener Name dort antwortet `NT_STATUS_OBJECT_PATH_NOT_FOUND`, ob es ihn gibt oder nicht; die eigene
  Freigabe bleibt wie zuvor erreichbar (`TestSambaPassageTellsNoGuessedNames`).
- **Ein Name wie ein lesbarer.** Samba verbirgt nach Namen, nicht nach Pfad: Wo der Benutzer eine Datei oder einen Ordner
  gleichen Namens (in jeder Schreibweise) in der Bibliothek liest, wird der durchschrittene Ordner nicht verborgen, weil
  das den lesbaren mit verbergen würde; dieser eine Name verrät einen geratenen Namen weiter, wie vor T-43, und das
  Protokoll sagt es („keeps telling a guessed name“). Ebenso ein Name mit `*`, `?`, `%`, `/` oder `\`, den eine
  Veto-Liste nicht fassen kann (`TestSambaPassageNameLikeAReadableOne`).

## Tests

- `scripts/check-samba.sh` (in der Pflichtprüfung der CI, lokal mit Docker: `sg docker -c scripts/check-samba.sh`): baut
  den Begleiter, startet einen für den Mandanten main und einen für acme und führt die Tests mit dem Tag `samba` per
  `smbclient` gegen sie aus: Ein Speichern wird eine Version, das Speichern von Office über ein Umbenennen behält die ID,
  Löschen geht in den Papierkorb, Rechte je Ordner halten, ein Benutzer ohne Rolle sieht nichts, ein falsches
  App-Passwort wird abgewiesen, ein entferntes App-Passwort hört sofort auf, und ein Benutzer eines Mandanten erreicht
  nie den anderen. Die Angriffsrunde vom 2026-11-20 (`attack_samba_test.go`) läuft dort auch: eine Flut von Auflistungen,
  ein harter Link, ein Verschieben mit alter ACL, Dateien, die die Vererbung brechen.
  Für die Domäne (T-42) startet es einen Domänencontroller von Samba mit erfundenen Konten und einem für den Lauf
  erzeugten Passwort (nie im Repository) und einen Begleiter für den Mandanten corp, der ihm beitritt. Das Skript prüft,
  dass der Beitritt stattfand, dass ein neuer Container auf demselben Volume ohne Passwortdatei startet, dass einer ohne
  Beitritt und ohne Passwortdatei endet und sagt, warum, und dass das Passwort in keinem Protokoll des Begleiters und in
  keinem seiner Prozesse steht (Befehlszeilen und Umgebungen). `samba_domain_test.go`: Ein Domänenkonto meldet sich per
  NTLMv2 und Kerberos an und speichert als sein Konto, auch eine Nummer der Schranke wird zugeordnet; ein Domänenkonto
  ohne Samba-Benutzer, `daemon`, der Administrator der Domäne, ein falsches Passwort und ein lokaler Benutzer des
  Begleiters werden abgewiesen; der Begleiter lehnt einen Zustand mit App-Passwörtern ab; eine UID, die von einem Konto
  an ein anderes übergeht, erreicht das Domänenkonto des ersten nicht.
- `scripts/t1-libreoffice.sh` (von Hand, es bindet eine Freigabe in einem Container ein): LibreOffice speichert über eine
  echte Freigabe, während das Protokoll von smbd jeden Vorgang aufzeichnet; die Aufnahmen in
  `internal/share/testdata/t1` spielt `TestRecordedSaves` in jedem Lauf ab (T-1).
