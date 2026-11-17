---
title: "max_connections in PostgreSQL"
collection: Fehlerkatalog
author: Anna Keller
tags: [fehler, postgresql, datenbank, einstellung]
changed: 2026-10-21
---
## Symptom

Die Einstellung max_connections begrenzt die Zahl gleichzeitiger Verbindungen.

## Ursache

Zu hohe Werte kosten Speicher, zu niedrige führen zu SQLSTATE 53300.

## Lösung

Wert nur zusammen mit shared_buffers ändern, Verbindungspool vorziehen. Aktueller Wert: 200.

## Betroffene Systeme

PostgreSQL 18
