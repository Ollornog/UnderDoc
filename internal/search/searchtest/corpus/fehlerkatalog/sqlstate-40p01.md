---
title: "SQLSTATE 40P01: Deadlock erkannt"
collection: Fehlerkatalog
author: Frank Berger
tags: [fehler, postgresql, datenbank, sperre]
changed: 2026-10-02
---
## Symptom

Nächtliche Buchungsläufe brechen gelegentlich mit SQLSTATE 40P01 ab.

## Ursache

Zwei Transaktionen sperren dieselben Zeilen in umgekehrter Reihenfolge.

## Lösung

Lauf wiederholen lassen; dauerhaft die Reihenfolge der Sperren vereinheitlichen. Siehe Runbook „Deadlocks analysieren“.

## Betroffene Systeme

PostgreSQL 18
