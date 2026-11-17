---
title: "Datenbankserver"
collection: IT-Dokumentation
author: David Nowak
tags: [datenbank, oracle, postgresql]
changed: 2026-02-05
---
## Oracle

db01 trägt die Warenwirtschaft (Oracle 19c). Listener auf dem Standardport. Fehler wie ORA-12541 und ORA-00942 stehen im Fehlerkatalog.

## PostgreSQL

db02 trägt die Kundendatenbank (PostgreSQL 18). max_connections steht auf 200, davor ein Verbindungspool.
