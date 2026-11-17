---
title: "Druckerwarteschlange leeren"
collection: Runbooks
author: Carla Weiß
tags: [runbook, drucker, windows]
changed: 2026-04-08
---
Hängt ein Druckauftrag, druckt nichts mehr. Das hilft auch bei Druckerfehler 79.

## Am Druckserver

Dienst Druckwarteschlange (Spooler) beenden, den Ordner C:\Windows\System32\spool\PRINTERS leeren, Dienst starten.

## Am Arbeitsplatz

In den Einstellungen unter Drucker die Warteschlange öffnen und alle Aufträge abbrechen.

## Bei 0x0000011b

Nicht die Warteschlange, sondern die Treiberverteilung ist schuld, siehe KB5005565.
