---
title: "Störungsbericht Webshop 02.02.2026"
collection: Protokolle
author: Anna Keller
tags: [störung, webshop, protokoll]
changed: 2026-06-15
---
## Ablauf

Der Webshop antwortete 40 Minuten lang mit HTTP 503, danach kurz mit HTTP 502.

## Ursache

Nach einem Update hielt die Anwendung ihre Datenbankverbindungen offen, PostgreSQL meldete SQLSTATE 53300.

## Maßnahmen

Verbindungspool eingeführt, max_connections nicht erhöht.
