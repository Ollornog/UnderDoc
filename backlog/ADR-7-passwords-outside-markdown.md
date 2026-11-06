---
id: ADR-7
type: Decision
title: Passwords never enter Markdown
status: erledigt
tags: [security]
created: 2026-11-06
---

## Context

Passwords should be writable in a page, hidden and revealed on click. Markdown sits on the share, in search, in versions, exports and backups.

## Options

- **Masked text in Markdown:** simple, but clear text everywhere.
- **Server-side encrypted store with a reference in the text:** convenient, but whoever holds server and key can read it.
- **End-to-end encryption with groups:** strongest, but a cryptography project of its own.

## Decision

The text holds only `::secret{id="..."}`. Values are encrypted in the database with a maintained library and a key from the environment. Each password has its own permission, values arrive from the server on click only, and every reveal is logged.

## Consequences

- Never included in search, exports, reports, public shares or the API.
- This is operations documentation, not a vault, and the documentation has to say so.
