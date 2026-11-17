---
title: "SQLSTATE 28P01: Passwort-Anmeldung fehlgeschlagen"
collection: Fehlerkatalog
author: Carla Weiß
tags: [fehler, postgresql, datenbank, anmeldung]
changed: 2026-07-14
---
## Symptom

Ein Dienst startet nicht, im Protokoll steht SQLSTATE 28P01.

## Ursache

Das Passwort des Datenbankbenutzers wurde geändert, der Dienst kennt noch das alte.

## Lösung

Passwort im Geheimnisspeicher des Dienstes aktualisieren und den Dienst neu starten.

## Betroffene Systeme

PostgreSQL 18
