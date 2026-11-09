---
id: ADR-20
type: Decision
title: Who sees and changes what, before roles arrive
status: erledigt
tags: [permissions]
created: 2026-11-07
---

## Context

The first build (M-28) brings accounts but not yet the roles per person and group of M-5. Something has to decide who sees and
changes what in the meantime. The maintainer asked for read-only and comment-only collections and folders, and left the rest to
this decision.

## Options

- **Everyone signed in may do everything:** simple, as in M-1, but nothing can be protected, and M-5 starts from nothing.
- **One level for all others, set per object and inherited:** small, and it is the "everyone" case of the roles M-5 brings, so
  nothing has to be thrown away.

## Decision

The second option.

- **Everyone past the gate sees the shared collections.** A private collection, such as My notes, and the private area are seen only
  by their user, also not by administrators.
- **Every object has an owner:** whoever made it. The owner and administrators can always edit.
- **For all others, an object says what they may do:** edit, comment or only read. The default is edit. A collection, folder or
  document passes its level down to what is below it, unless something sets its own.
- **Read only** means no change, no new child, no move in or out, and no comment. **Comment** adds comments to read only.
- The level shows as a lock at the heading of a collection and as a note at the document, and what it forbids is greyed out
  (ADR-17).
- **Only the owner sets the level (2026-11-09):** someone who may edit cannot shut the others out. It is set in the overview of a
  collection and in the settings of a document, and the database keeps it; below the top "as above" takes it from there.

## Consequences

- M-5 replaces the one level for all others with roles per person and group, and keeps the owner.
- Comments arrive with the first build, each on an element of a page through its ID (ADR-18).
