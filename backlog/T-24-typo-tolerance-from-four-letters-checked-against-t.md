---
type: Task
title: Typo tolerance from four letters, checked against the question catalogue
status: erledigt
id: T-24
milestone: M-12
created: '2026-11-23'
---
# T-24 — Typo tolerance from four letters, checked against the question catalogue

Search allows a typo from five letters today. A team wiki on the same engine allows one from four letters, because short words such as 'Kasa' for 'Kassa' found nothing otherwise. Measure both settings against the question catalogue of M-12, and keep the one that finds more without adding noise. Codes stay exact.

**Done 2026-11-23** ([docs/search.md](../docs/search.md#typing-errors-from-four-letters-t-24-2026-11-23)): four letters kept for both engines, measured with eight questions of the kind `typo-short` (Meilisearch MRR 0.000 → 1.000, OpenSearch 0.823 → 1.000), first hits of short words as written unchanged, codes exact; the floor 0.95 in the corpus tests holds it.
