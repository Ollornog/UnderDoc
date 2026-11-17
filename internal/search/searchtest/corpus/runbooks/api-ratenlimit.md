---
title: "Ratenlimit der API"
collection: Runbooks
author: Anna Keller
tags: [runbook, api, http]
changed: 2026-04-20
---
Der Marktplatz erlaubt 120 Anfragen pro Minute. Darüber antwortet er mit HTTP 429.

## Vorgehen

Bestandsabgleich in Blöcken zu 50 Artikeln senden, Retry-After beachten, nachts den Vollabgleich fahren.
