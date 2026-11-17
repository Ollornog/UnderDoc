---
title: "Firewall-Regelwerk"
collection: IT-Dokumentation
author: David Nowak
tags: [firewall, netzwerk, sicherheit]
changed: 2026-03-25
---
## Ausgehend

| Quelle | Ziel | Port | Zweck |
|---|---|---|---|
| Büro | Internet | 443 | Web |
| Kassennetz | Zahlungsdienstleister | 443 | Kartenzahlung |
| app01 | Marktplatz | 443 | Bestandsabgleich |

## Eingehend

Nur der Webshop über den Reverse Proxy (Port 443). Verwaltungszugänge nur über das VPN.
