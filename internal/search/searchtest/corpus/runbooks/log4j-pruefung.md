---
title: "Log4j-Prüfung"
collection: Runbooks
author: David Nowak
tags: [runbook, sicherheit, cve, java]
changed: 2026-05-26
---
Prüfung aller Systeme auf CVE-2021-44228.

## Suchen

Auf jedem Server nach log4j-core-*.jar suchen, auch in ZIP- und WAR-Dateien.

## Bewerten

Versionen vor 2.17 aktualisieren oder die Anwendung beim Hersteller nachfragen.
