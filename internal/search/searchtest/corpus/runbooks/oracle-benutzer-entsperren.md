---
title: "Oracle-Benutzer entsperren"
collection: Runbooks
author: Elif Yılmaz
tags: [runbook, oracle, datenbank, anmeldung]
changed: 2026-06-10
---
Nach mehreren ORA-01017 sperrt die Datenbank das Konto mit ORA-28000.

## Entsperren

Als DBA: ALTER USER name ACCOUNT UNLOCK; bei Bedarf neues Passwort setzen.

## Ursache

Meist ein Dienst mit altem Passwort, der es immer wieder versucht.
