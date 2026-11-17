---
title: "Deadlocks analysieren"
collection: Runbooks
author: Bernd Hofmann
tags: [runbook, postgresql, datenbank, sperre]
changed: 2026-02-07
---
## Erkennen

SQLSTATE 40P01 im Protokoll der Anwendung, im Protokoll von PostgreSQL steht der Deadlock mit beiden Abfragen.

## Abhilfe

Zeilen immer in derselben Reihenfolge sperren (nach Primärschlüssel sortiert), Transaktionen kurz halten.
