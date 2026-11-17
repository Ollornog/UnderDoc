---
title: "rsync error code 23: Teilweise übertragen"
collection: Fehlerkatalog
author: Carla Weiß
tags: [fehler, backup, nas, linux]
changed: 2026-08-12
---
## Symptom

Die nächtliche Kopie auf das NAS endet mit rsync error code 23.

## Ursache

Einige Dateien waren nicht lesbar, meist wegen fehlender Rechte oder geöffneter Dateien.

## Lösung

Im Protokoll die betroffenen Dateien suchen und Rechte prüfen.

## Betroffene Systeme

NAS-Sicherung
