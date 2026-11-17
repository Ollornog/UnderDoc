---
title: "HTTP 503: Dienst nicht verfügbar"
collection: Fehlerkatalog
author: Bernd Hofmann
tags: [fehler, webshop, http, störung]
changed: 2026-07-13
---
## Symptom

Der Webshop zeigt „Service Unavailable“ mit dem Status HTTP 503.

## Ursache

Die Anwendung hinter dem Proxy läuft nicht oder ist überlastet; während eines Deployments ist das kurz normal.

## Lösung

Prüfen, ob ein Deployment läuft. Sonst den Anwendungsdienst neu starten und die Last im Monitoring ansehen. Siehe Runbook „Webshop nach Ausfall wiederherstellen“.

## Betroffene Systeme

Webshop, Reverse Proxy
