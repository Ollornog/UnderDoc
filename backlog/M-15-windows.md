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

- Every link and list opens in the main window, in the general tab, which the next document replaces.
- The general tab is always first and looks different from the others.
- A button keeps the document as a tab of its own at once. Opening a document that has a tab there jumps to it.
- Dragging sorts tabs and moves them to another window. A tab can also be duplicated and closed.
- Dragging a tab to the right or left edge opens a new column on that side, up to four, and a button and the tab's menu do the same.
  A window disappears with its last tab, and the main window stays.
- Each tab has its own scroll position, and the same document can be open in two tabs and two windows.
- Each tab offers find on the page, with the field right at the magnifier, and a source view. A status line counts words,
  characters and lines.
- The layout is part of the URL, so it survives a reload and can be sent as a link.
- A phone shows one window at a time.
- **Acceptance: a browser test opens four windows, one document twice, sorts tabs by dragging, and each window scrolls and navigates
  on its own.**
