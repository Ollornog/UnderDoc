---
id: ADR-3
type: Decision
title: Files in folders are the source of truth
status: verworfen
superseded_by: ADR-13
tags: [architecture, excel]
created: 2026-11-06
---

## Context

People keep using Excel and Explorer. Pages should stay plain Markdown that outlives the application.

## Options

- **Everything in the database:** consistent and easy for real-time, but the share would only be an export.
- **Files as truth, database as index:** works with Excel and any editor, but the app must handle outside changes.

## Decision

Files are the truth for pages, media, values and snippets. The database holds the index, cache, permissions and whatever cannot be a file, such as native lists and passwords.

## Consequences

- The watcher has to cope with how Office saves, and moves need rename detection.
- A real-time editor later collides with outside changes (M-10).
