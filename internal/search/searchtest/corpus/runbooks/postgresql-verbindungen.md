---
title: "PostgreSQL-Verbindungen begrenzen"
collection: Runbooks
author: Carla Weiß
tags: [runbook, postgresql, datenbank]
changed: 2026-03-21
---
Bei SQLSTATE 53300 sind alle Verbindungen belegt.

## Sofort

Mit pg_stat_activity hängende Sitzungen finden und mit pg_terminate_backend beenden.

## Dauerhaft

Verbindungspool (PgBouncer) vor die Datenbank stellen. max_connections nicht einfach erhöhen, jede Verbindung kostet Speicher.
