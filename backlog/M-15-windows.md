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

- Every link and list opens in the general tab, which the next document replaces.
- The general tab is always the first tab of the leftmost window, looks different from the others and cannot be dragged.
- An icon in the general tab opens its document as a tab of its own at once. Opening a document that has a tab there jumps to it.
- Dragging sorts tabs and moves them to another window, and a dragged tab snaps into its place while the others make room. A tab can
  also be duplicated and closed. Tabs shrink like browser tabs, so none hides beyond the edge.
- Dragging a tab to the left or right third of a window opens a new column on that side, up to four, and a preview shows the result
  first. A button and the menu do the same. A column on the far left takes the general tab along.
- The active window has a clear frame. A window disappears with its last tab, except the leftmost one. Windows have no numbers.
- Each tab has its own scroll position, and the same document can be open in two tabs and two windows.
- All buttons for a document sit on one level below the tabs. Find on the page has its field right at the magnifier, and a status line
  counts words, characters and lines.
- The layout is part of the URL, so it survives a reload and can be sent as a link.
- A phone shows one window at a time.
- **Acceptance: a browser test with real pointer input opens four windows, one document twice, sorts tabs by dragging, and each window
  scrolls and navigates on its own.**
