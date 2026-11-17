---
title: "DNS-Probleme beheben"
collection: Runbooks
author: Frank Berger
tags: [runbook, dns, netzwerk]
changed: 2026-07-13
---
## Symptom

Der Browser meldet DNS_PROBE_FINISHED_NXDOMAIN, andere Seiten gehen.

## Prüfen

nslookup name gegen den internen DNS-Server. ipconfig /flushdns. DNS-Server des Rechners prüfen (per DHCP).

## Interne Namen

Interne Namen werden nur im Firmennetz und über das VPN aufgelöst.
