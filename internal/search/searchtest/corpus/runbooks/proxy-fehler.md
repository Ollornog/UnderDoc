---
title: "Proxy-Fehler beheben"
collection: Runbooks
author: Frank Berger
tags: [runbook, proxy, netzwerk, browser]
changed: 2026-09-01
---
## Symptome

ERR_CONNECTION_REFUSED im Browser, Windows Update mit 0x8024402c.

## Prüfen

Proxy-Einstellung per Gruppenrichtlinie; manuelle Einträge entfernen. netsh winhttp reset proxy.
