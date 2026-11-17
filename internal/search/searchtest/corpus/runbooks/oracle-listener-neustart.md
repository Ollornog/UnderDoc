---
title: "Oracle-Listener neu starten"
collection: Runbooks
author: Elif Yılmaz
tags: [runbook, oracle, datenbank, listener]
changed: 2026-03-08
---
Wenn Clients ORA-12541 melden, läuft der Listener nicht.

## Status prüfen

Als Benutzer oracle: lsnrctl status.

## Starten

lsnrctl start. Danach mit tnsping vom Client prüfen.

## Dauerhaft

Den Dienst so einrichten, dass er mit dem Server startet. Im Protokoll listener.log nach der Ursache suchen.
