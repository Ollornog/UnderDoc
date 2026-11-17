---
title: "Windows-Updates manuell installieren"
collection: Runbooks
author: Frank Berger
tags: [runbook, windows, update]
changed: 2026-02-02
---
Wenn Windows Update immer wieder scheitert, etwa mit 0x80070005 oder 0x8024402c, wird das Update von Hand installiert.

## Update-Cache zurücksetzen

Als Administrator: Dienste wuauserv und bits beenden, den Ordner SoftwareDistribution umbenennen, Dienste starten.

## Paket herunterladen

Das Paket (zum Beispiel KB5034441) aus dem Microsoft Update-Katalog laden und mit wusa.exe installieren.

## Prüfen

Nach dem Neustart unter Updateverlauf prüfen, ob das Update installiert ist.
