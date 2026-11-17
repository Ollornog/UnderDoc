---
title: "TLS-Zertifikat erneuern"
collection: Runbooks
author: Bernd Hofmann
tags: [runbook, zertifikat, tls, webserver]
changed: 2026-04-02
---
Läuft ein Zertifikat ab, zeigen Browser ERR_CERT_DATE_INVALID.

## Automatisch erneuerte Zertifikate

Die meisten Zertifikate erneuert der Proxy selbst über ACME. Bleibt das aus, Protokoll des Proxys prüfen (DNS-Eintrag, Port 80 erreichbar).

## Gekaufte Zertifikate

Neue Zertifikatsanforderung (CSR) erzeugen, beim Anbieter einreichen, Zertifikat und Zwischenzertifikat einspielen, Dienst neu laden.

## Prüfen

Ablaufdatum im Browser prüfen und in der Zertifikatsübersicht eintragen.
