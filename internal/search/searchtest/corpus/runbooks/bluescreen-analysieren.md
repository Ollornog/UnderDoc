---
title: "Bluescreen analysieren"
collection: Runbooks
author: Frank Berger
tags: [runbook, windows, bluescreen]
changed: 2026-01-16
---
## Stoppcode lesen

Den Stoppcode notieren, zum Beispiel 0xc000021a. Im Systemprotokoll steht danach meist Event ID 41.

## Nach Updates

Startet der Rechner nach KB5021233 nicht mehr, in der Wiederherstellungsumgebung das letzte Update deinstallieren.

## Speicherabbild

Die Datei MEMORY.DMP mit WinDbg auswerten oder an den Support geben.
