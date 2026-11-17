---
title: "HTTP 504: Gateway Timeout"
collection: Fehlerkatalog
author: Anna Keller
tags: [fehler, webshop, http]
changed: 2026-02-13
---
## Symptom

Lange Exporte im Webshop enden mit HTTP 504.

## Ursache

Die Anwendung braucht länger als das Zeitlimit des Proxys.

## Lösung

Exporte als Hintergrundauftrag starten statt im Browser warten zu lassen; Zeitlimit nur begründet erhöhen.

## Betroffene Systeme

Webshop, Reverse Proxy
