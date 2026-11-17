---
title: "Backup-Fehler VSS beheben"
collection: Runbooks
author: David Nowak
tags: [runbook, backup, vss, windows]
changed: 2026-07-15
---
Die Sicherung der Windows-Server meldet 0x800423F4.

## Diagnose

vssadmin list writers ausführen. Ein Writer mit Status „Fehler“ zeigt die betroffene Anwendung.

## Beheben

Den zugehörigen Dienst neu starten (zum Beispiel SQL Server VSS Writer) und die Sicherung von Hand starten.

## Wenn es bleibt

Neustart des Servers im Wartungsfenster.
