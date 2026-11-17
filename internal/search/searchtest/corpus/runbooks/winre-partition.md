---
title: "WinRE-Partition vergrößern"
collection: Runbooks
author: Elif Yılmaz
tags: [runbook, windows, winre, update]
changed: 2026-07-13
---
Hintergrund: KB5034441 braucht 250 MB freien Platz in der Wiederherstellungspartition und scheitert sonst mit 0x80070643.

## Vorgehen

Mit reagentc /info die Partition finden. reagentc /disable. Die Systempartition mit diskpart um 250 MB verkleinern, die Wiederherstellungspartition löschen und neu anlegen, reagentc /enable.

## Prüfen

reagentc /info zeigt „Enabled“. Danach das Update erneut anbieten lassen.

## Vorsicht

Vorher eine Sicherung des Rechners machen. Auf Rechnern mit BitLocker zuerst den Wiederherstellungsschlüssel prüfen.
