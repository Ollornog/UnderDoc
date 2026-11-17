---
title: "ERR_CERT_DATE_INVALID: Zertifikat abgelaufen"
collection: Fehlerkatalog
author: Bernd Hofmann
tags: [fehler, browser, zertifikat, tls]
changed: 2026-09-23
---
## Symptom

Der Browser warnt mit NET::ERR_CERT_DATE_INVALID.

## Ursache

Das TLS-Zertifikat des Servers ist abgelaufen, oder die Uhr des Rechners geht falsch.

## Lösung

Uhrzeit des Rechners prüfen. Ist das Zertifikat abgelaufen, das Runbook „TLS-Zertifikat erneuern“ befolgen.

## Betroffene Systeme

Webserver, Browser
