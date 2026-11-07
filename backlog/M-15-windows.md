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

- Every window has its own general tab, always its first tab. Every link and list opens in the general tab of the active window, and
  the next document replaces it.
- The general tab looks different from the others and only exists while it holds a document. One click selects it. What was opened
  in it before shows as a stack of tabs below it, and its arrow lists the last 15.
- When the general tab gives up its document, by closing it, keeping it or dragging it to a place of its own, the one opened before
  takes its place. Opening a document that has a tab in that window jumps to it.
- The tabs sit in the top row, beside the name and the search field above the columns.
- Dragging sorts tabs and moves them to another window, and a dragged tab snaps into its place while the others make room.
- A right click on a tab and the dots below the tabs show the same menu: close, close others, close to the right or left, close all,
  duplicate or keep, copy path, move, and close the window. A middle click closes a tab, a double click on the empty part of the row
  opens New, and a button lists all tabs of the window. With more than one window, each can be closed with all its tabs. Tabs shrink like browser tabs, so none hides beyond the edge.
- Dragging a tab to the left or right third of a window opens a new column on that side, up to four, and a preview shows the result
  first. A button and the menu do the same. A new column brings its own general tab.
- The active window has a line along its top, and the others step back a little. A window disappears when only its empty general
  tab is left, unless it is the last one. Windows have no numbers.
- Each tab has its own scroll position, and the same document can be open in two tabs and two windows.
- All buttons for a document sit on one level below the tabs. Find on the page has its field right at the magnifier, and a status line
  counts words, characters and lines.
- The layout is part of the URL, so it survives a reload and can be sent as a link.
- A phone shows one window at a time.
- **Acceptance: a browser test with real pointer input opens four windows, one document twice, sorts tabs by dragging, drags a general tab
  into another window, and each window scrolls and navigates on its own.**
