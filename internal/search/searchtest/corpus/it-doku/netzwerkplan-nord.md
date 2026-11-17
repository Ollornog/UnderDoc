---
title: "Netzwerkplan Filiale Nord"
collection: IT-Dokumentation
author: Elif Yılmaz
tags: [netzwerk, filiale]
changed: 2026-08-09
---
Die Filiale Nord hat drei Netze, getrennt durch VLANs.

## VLANs

| VLAN | Zweck | Netz |
|---|---|---|
| 10 | Büro | 192.0.2.0/26 |
| 20 | Kassen und Kartenterminals | 192.0.2.64/27 |
| 30 | Gäste-WLAN | 198.51.100.0/24 |

## Router

Der Router steht im Technikschrank im Lager. Er baut den VPN-Tunnel zur Zentrale auf.

## Kassennetz

Kassen und Kartenterminals sprechen nur mit der Zentrale und dem Zahlungsdienstleister.
