---
title: "SMTP 550 5.7.1: Weiterleitung abgelehnt"
collection: Fehlerkatalog
author: Frank Berger
tags: [fehler, mail, smtp]
changed: 2026-02-01
---
## Symptom

Mails an externe Empfänger kommen mit „550 5.7.1 Relaying denied“ zurück.

## Ursache

Der sendende Server darf über das Relay nicht nach außen senden, oder die Absenderadresse passt nicht zum Konto.

## Lösung

Absenderadresse prüfen; für Geräte wie Scanner den internen Relay-Connector nutzen.

## Betroffene Systeme

Mailserver, Scanner
