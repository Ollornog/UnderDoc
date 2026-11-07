---
id: ADR-18
type: Decision
title: Object IDs and addresses
status: offen
tags: [architecture, spot]
created: 2026-11-07
---

## Context

Everything is an object with a stable ID, a URL and a way to be embedded elsewhere (ADR-17). That includes parts of a document,
such as a table or a paragraph used on another page. Many objects mean many IDs, and the question is how to keep them under
control. One idea was to derive an ID from its parent, so it shows where an object comes from.

M-1 already gives every page an ID: an `id:` chosen by hand in its front matter, or its path until it has one. Documents open at
`/d/<id>`.

## Options

- **The path or place as the ID:** readable, but a move changes it and breaks every link. ADR-3 was dropped for this reason.
- **IDs derived from the parent, such as `ops.network.plan`:** they show where an object comes from, but after a move they either
  lie or change, and a changed ID breaks every link to it.
- **Flat IDs for objects, and IDs for parts that are local to their object:** an object's ID never changes, and a part is
  addressed through the object it lives in. Where something comes from is shown by its place in the workspace, not by its ID.

## Decision

Proposed: the third option. The maintainer confirms it in the talk for M-28.

- **Objects get a short random ID** when they are created, such as `7k2m9q4x1c`. It never changes and is never given out twice,
  whatever the kind of object. An `id:` chosen by hand in the front matter stays valid, as long as it is unique.
- **One address for every object:** `/o/<id>` opens it, whatever its kind. A readable name may follow, as in
  `/o/7k2m9q4x1c/network-plan`, but only the ID counts, so a renamed object keeps working links. Links to `/d/<id>` from M-1 keep
  working.
- **Parts get an ID local to their document**, addressed as `<object id>#<part id>`, such as `/o/7k2m9q4x1c#t4`. A part lives in
  exactly one document and moves with it, so here the nesting is true and the address may show it.
- **A part gets its ID the first time something refers to it**, through copy link or embedding, and keeps it from then on. In
  Markdown it is a short marker at the end of the part, so the file stays readable. Parts nobody refers to carry no marker.
- **Object or part:** something with a life of its own, such as a table edited in the web, a list or a drawing, is an object.
  It sits below the page as a child and is embedded there. Text, headings and Markdown tables are parts of the page.
- **Nobody types IDs.** Links come from search while typing, from copy link and from dragging. The app keeps every reference in
  both directions: each object and part shows where it is used, a list names broken references (M-3), and deleting something in
  use asks first.
- **The place is a record of its own** (ADR-13): an object knows its parent and its collection. That costs one entry per object,
  so there is no reason to put the parent into the ID.

## Consequences

- The ID generator and the reference index need the database, so they come with M-28.
- On the phone, the same URL opens the same object (ADR-17).
- Still open:
  - Whether entries of the private area, such as to-dos, events and time records, are objects with addresses too, visible only to
    their user, or records inside one private object per kind (M-24).
  - The exact form of the marker for parts in Markdown, and whether embedding a part needs a stable heading or paragraph at all
    (M-3).
