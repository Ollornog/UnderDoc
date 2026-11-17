---
title: "HTTP 429: Zu viele Anfragen"
collection: Fehlerkatalog
author: David Nowak
tags: [fehler, api, http]
changed: 2026-04-02
---
## Symptom

Die Anbindung an den Marktplatz meldet HTTP 429.

## Ursache

Das Ratenlimit der Schnittstelle ist erreicht.

## Lösung

Abfragen bündeln und die Pause aus dem Header Retry-After einhalten. Siehe Runbook „Ratenlimit der API“.

## Betroffene Systeme

Marktplatz-Anbindung
