---
id: M-15
type: Milestone
title: "Windows and tabs: documents side by side"
status: offen
blocked_by: [M-14]
tags: [ui]
created: 2026-11-06
---

Documents open in tabs, and tabs split off into windows, so documents can be read next to each other (ADR-16).

**Done when this is true:**

- Everything opens in the first window, in a general tab that the next document replaces.
- Pinning a tab keeps it, and the next document opens in a new general tab. Opening a document that has a tab there jumps to it.
- A tab can be moved to another window, duplicated and closed.
- A tab split off by dragging it to the right edge, with a button or from its menu makes a new window, up to four as columns.
  A window disappears with its last tab.
- Each tab has its own scroll position, and the same document can be open in two tabs and two windows.
- Each tab offers find on the page and a source view, and a status line counts words, characters and lines.
- The layout is part of the URL, so it survives a reload and can be sent as a link.
- A phone shows one window at a time.
- **Acceptance: a browser test opens four windows, one document twice, and each one scrolls and navigates on its own.**
