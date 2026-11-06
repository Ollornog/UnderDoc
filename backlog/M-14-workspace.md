---
id: M-14
type: Milestone
title: "Workspace: tabs, tree, page widths and facts"
status: offen
blocked_by: [M-1]
tags: [ui, shell]
created: 2026-11-06
---

The shell that later modules plug into (ADR-14). It needs no database, so it runs on the read-only wiki. T-5 settles how
it looks before anything is built.

**Done when this is true:**

- A sticky top bar holds the tabs of the areas, for example Documents and Data. A module with nothing to show adds no tab.
- The left navigation stays, in a slimmer form.
- Collections sit above the tree. Folders open by themselves up to a set number of children, and what the reader opens or
  closes wins.
- A document can recommend a width in its front matter: slim, wide or full, where full hides the rail. The reader can override it.
- Every document has a facts panel with name, location, last change, external links, internal links and backlinks.
- Modules add tabs and facts panel sections through one small interface, proven by a test module.
- **Acceptance: the chosen variant runs on real content, and a browser test covers it at desktop and phone width.**
