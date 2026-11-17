---
title: "E-Mail-Konfiguration"
collection: IT-Dokumentation
author: Bernd Hofmann
tags: [mail, konzept, dns]
changed: 2026-09-18
---
## DNS

SPF erlaubt nur den Mailserver und den Newsletterdienst. DKIM signiert alle ausgehenden Mails, DMARC steht auf quarantine.

## Clients

IMAP 993 mit TLS, Senden über 587 mit STARTTLS.
