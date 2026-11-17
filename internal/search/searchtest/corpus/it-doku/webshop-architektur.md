---
title: "Webshop-Architektur"
collection: IT-Dokumentation
author: Elif Yılmaz
tags: [webshop, konzept]
changed: 2026-05-21
---
## Aufbau

Reverse Proxy vor der Anwendung, Anwendung auf app01, Daten in PostgreSQL auf db02, Bilder im Objektspeicher.

## Schnittstellen

Bestand und Preise kommen aus der Warenwirtschaft, der Marktplatz wird stündlich abgeglichen.
