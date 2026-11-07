---
id: M-24
type: Milestone
title: Private area
status: offen
blocked_by: [M-5]
tags: [module, people]
created: 2026-11-07
---

A view in nav1 that only its user can see, as a module an administrator can switch off. Only an idea so far: the talk before this
milestone (ADR-14) settles the scope.

**To settle in that talk:**

- Which parts come first: to-dos, calendar, time clock, vacation planning, notes.
- Whether time records and vacation stay purely private notes, or feed something official. Records about work are personal data and
  can need an agreement with employee representatives, so this needs its own look first.
- How a private calendar relates to the shared calendar (M-18), and private to-dos to lists (M-6).

**Done when this is true:**

- Nobody else can see the content, including administrators through the interface, and that is stated plainly in the view.
- To-dos, calendar, time clock, vacation planning and notes each open in a tab like a document.
- A user can export and delete all of their private data.
- Switched off, the module leaves no entry behind, and no data is touched.
