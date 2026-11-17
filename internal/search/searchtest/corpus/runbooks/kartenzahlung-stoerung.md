---
title: "Störung der Kartenzahlung beheben"
collection: Runbooks
author: Frank Berger
tags: [runbook, kasse, kartenzahlung, störung]
changed: 2026-03-24
---
Dieses Runbook gilt, wenn an einer oder mehreren Kassen keine Kartenzahlung möglich ist.

## Erste Prüfung

Fehlercode an der Kasse ablesen. E-1042 heißt, die Kasse erreicht das Terminal nicht. E-1043 heißt, das Terminal erreicht den Netzbetreiber nicht. E-1044 ist eine Ablehnung durch die Bank und keine Störung.

## Eine Kasse betroffen

Terminal neu starten, Kabel prüfen, Terminal-Diagnose drucken. Bleibt E-1042, Terminal mit dem einer anderen Kasse tauschen.

## Alle Kassen einer Filiale betroffen

Internetleitung der Filiale prüfen, Router neu starten. Statusseite des Zahlungsdienstleisters ansehen. Kundschaft auf Barzahlung hinweisen und das Schild „Zurzeit nur Barzahlung“ aufstellen.

## Nachbereitung

Störung mit Uhrzeit und Fehlercode im Störungsbericht festhalten.
