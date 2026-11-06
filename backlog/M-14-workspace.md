---
id: M-14
type: Milestone
title: "Workspace: three views, hierarchy, page widths and facts"
status: offen
blocked_by: [M-1]
tags: [ui, shell]
created: 2026-11-06
---

The shell that later modules plug into (ADR-14), with the model of ADR-15. It needs no database, so it runs on the read-only
wiki, with pages as the first document type. T-5 settles how it looks before anything is built.

**Done when this is true:**

- A sticky top bar holds the tabs. A module with nothing to show adds no tab.
- The Workspace tab keeps the left navigation, in a slimmer form, with collections above the tree.
- A page can have subpages. `page.md` next to a folder `page/` appears as one document with children.
- Folders look different from documents in the navigation, with separators or extra spacing, and work like documents otherwise.
- Documents open by themselves up to a set number of children, and what the reader opens or closes wins.
- The Files tab shows the folder structure on disk, including images.
- A document's width is part of its metadata, in the front matter: slim (the default), wide or full, where full hides the rail.
  Trying another width in the view changes nothing stored. Saving it from the web comes with editing.
- Every document has a facts panel with name, type, width, place in the workspace, path on disk, last change, external links,
  internal links and backlinks.
- Modules add document types, tabs and facts panel sections through one small interface, proven by a test module.
- **Acceptance: the chosen variant runs on real content, and a browser test covers it at desktop and phone width.**
