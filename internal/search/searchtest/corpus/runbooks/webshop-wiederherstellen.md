---
title: "Webshop nach Ausfall wiederherstellen"
collection: Runbooks
author: Frank Berger
tags: [runbook, webshop, störung, http]
changed: 2026-06-11
---
## Lage erfassen

Antwortet der Webshop mit HTTP 503, läuft die Anwendung nicht. HTTP 502 heißt, der Proxy erreicht sie nicht. HTTP 504 heißt, sie antwortet zu langsam.

## Anwendung neu starten

Auf dem Anwendungsserver den Dienst webshop neu starten und das Protokoll beobachten.

## Datenbank prüfen

Kommt SQLSTATE 53300 im Protokoll vor, zuerst die Verbindungen bereinigen.

## Kommunikation

Bei mehr als 15 Minuten Ausfall die Wartungsseite schalten und den Kundenservice informieren.
