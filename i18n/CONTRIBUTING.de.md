# Mitwirken

<a href="../CONTRIBUTING.md">English</a> · <b>Deutsch</b>
<br /><br />

Danke, dass du dir die Zeit nimmst. Das Projekt ist klein, die Regeln sind kurz.

## Bevor du Code schreibst

**Öffne zuerst ein Issue**, wenn es um mehr als einen Tippfehler oder einen offensichtlichen Bug geht. Die Gründe
hinter dem Entwurf stehen in [`backlog/`](../backlog/) als Entscheidungen, lies bitte die passende, bevor du sie ändern willst.

**Bring eigenen Code mit.** UnderDoc teilt Ziele mit anderen Wikis, nicht ihren Code. Kopiere oder portiere keinen
Code, keine Texte, Assets oder Übersetzungen von dort, vor allem nicht aus Projekten unter einer Source-Available- oder Enterprise-Lizenz.

## Ablauf

- Arbeite auf einem **Feature-Branch**, nicht auf `main`.
- Fahre vor dem Push die volle Suite: `scripts/check.sh`. Sie muss **zweimal hintereinander** grün sein.
- Doku und `CHANGELOG.md` gehören in **denselben Commit** wie die Änderung, die sie beschreiben.
- Code, Kommentare, Commits und Doku sind Englisch. Deutsch gibt es nur als Übersetzung.
- Beispiele nutzen `example.com` und erfundene Namen, nie echte Firmen, Hosts oder Daten.

## Was die Suite prüft

Neben den Go-Tests prüft die Suite die Repo-Hygiene: Pflichtdateien, gepinnte GitHub Actions, Workflow-Rechte,
CHANGELOG-Aufbau und gleiche Gestalt der deutschen und englischen Dokumente. Schlägt sie an, behebe die Ursache,
statt die Prüfung zu umgehen.

## Sicherheitsrelevante Änderungen

Alles an Rechten, Teilen, Quellen oder Passwörtern braucht einen Test, der das behobene Problem gefunden hätte.
Hast du eine Schwachstelle gefunden, folge bitte [SECURITY](SECURITY.de.md), statt einen Pull Request zu öffnen.

## Verhaltenskodex

Mit deiner Mitwirkung stimmst du dem [Verhaltenskodex](CODE_OF_CONDUCT.de.md) zu.

<br /><br />
<p align="right"><img src="../docs/logo.svg" alt="UnderDoc" width="60" height="60"></p>
