---
title: "Server im Wartungsfenster neu starten"
collection: Runbooks
author: Frank Berger
tags: [runbook, server, wartung]
changed: 2026-01-09
---
## Wartungsfenster

Dienstag 21 bis 23 Uhr, nach Ankündigung im Kanal „IT-Info“ mindestens zwei Tage vorher.

## Reihenfolge

Zuerst Anwendungsserver, dann Datenbankserver, zuletzt Domänencontroller einzeln. Nach jedem Neustart Dienste prüfen.

## Prüfen

Monitoring muss grün sein. Event ID 7034 im Systemprotokoll nach dem Start prüfen.
