---
title: "SQLSTATE 23505: Eindeutigkeit verletzt"
collection: Fehlerkatalog
author: Frank Berger
tags: [fehler, postgresql, datenbank, import]
changed: 2026-04-12
---
## Symptom

Der Import der Kundendaten in PostgreSQL bricht mit SQLSTATE 23505 (unique_violation) ab.

## Ursache

Ein Datensatz mit demselben Schlüssel ist schon vorhanden, meist eine doppelte Kundennummer in der Importdatei.

## Lösung

Importdatei auf doppelte Kundennummern prüfen; für wiederholte Importe INSERT … ON CONFLICT verwenden.

## Betroffene Systeme

PostgreSQL 18 der Kundendatenbank
