---
title: "SQLSTATE 53300: Zu viele Verbindungen"
collection: Fehlerkatalog
author: Frank Berger
tags: [fehler, postgresql, datenbank]
changed: 2026-07-19
---
## Symptom

Anwendungen bekommen keine Verbindung mehr, PostgreSQL meldet SQLSTATE 53300 (too_many_connections).

## Ursache

max_connections ist erreicht, meist weil eine Anwendung ihre Verbindungen nicht zurückgibt.

## Lösung

Verbindungspool der Anwendung prüfen, hängende Sitzungen beenden. Siehe Runbook „PostgreSQL-Verbindungen begrenzen“.

## Betroffene Systeme

PostgreSQL 18
