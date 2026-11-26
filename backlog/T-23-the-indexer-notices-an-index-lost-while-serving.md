---
type: Task
title: The indexer notices an index lost while serving
status: offen
sprint: S2
points: 2
id: T-23
milestone: M-12
created: '2026-11-23'
---
# T-23 — The indexer notices an index lost while serving

The indexer checks whether the index exists only when `serve` starts (Prepare). When the search service loses its data while UnderDoc runs (volume replaced, service restored from an older state), UnderDoc keeps writing into a fresh empty index and finds only what changed since; docs/services.md says to restart UnderDoc. Wanted: the indexer notices a missing or reset index while running (for example a cheap check of the index and its version with every round, or when a write answers index_not_found) and fills it again from the queue, as at start. Test: drop the index under a running indexer, the index is full again without a restart. Found in the Meilisearch 1.57 upgrade (#143).
