---
title: "HTTP 502: Bad Gateway"
collection: Fehlerkatalog
author: Anna Keller
tags: [fehler, webshop, http, proxy]
changed: 2026-08-18
---
## Symptom

Der Webshop oder das Intranet antwortet mit HTTP 502.

## Ursache

Der Reverse Proxy erreicht die Anwendung nicht: falscher Port, Anwendung abgestürzt oder Firewall dazwischen.

## Lösung

Anwendung und Port prüfen, Protokoll des Proxys lesen.

## Betroffene Systeme

Reverse Proxy
