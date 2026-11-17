---
title: "Netzwerkdrucker einrichten"
collection: Runbooks
author: Bernd Hofmann
tags: [runbook, drucker, netzwerk]
changed: 2026-07-22
---
## Voraussetzungen

Der Drucker hat eine feste IP-Adresse aus dem Druckernetz und steht in der Druckerliste.

## Am Druckserver

Drucker mit Treiberpaket (Typ 4) anlegen, freigeben und in der Gruppenrichtlinie „Drucker Filiale“ zuweisen.

## Am Arbeitsplatz

Nach der Anmeldung erscheint der Netzwerkdrucker automatisch. Nicht über „Drucker hinzufügen“ von Hand verbinden, das führt seit KB5005565 zu 0x0000011b.
