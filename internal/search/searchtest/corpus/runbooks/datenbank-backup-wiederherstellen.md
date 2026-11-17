---
title: "Datenbank-Backup wiederherstellen"
collection: Runbooks
author: Carla Weiß
tags: [runbook, datenbank, backup, postgresql]
changed: 2026-08-13
---
## Wann

Nach Datenverlust, beschädigten Tabellen oder für eine Testkopie.

## Vorbereitung

Anwendungen stoppen, damit niemand schreibt. Den gewünschten Sicherungsstand im Backup-Katalog auswählen.

## Wiederherstellen

Mit pg_restore in eine neue Datenbank zurückspielen, Inhalt prüfen, dann umbenennen. Nie über die laufende Datenbank spielen.

## Danach

Anwendungen starten und Stichproben prüfen. Wiederherstellung im Änderungsprotokoll vermerken.
