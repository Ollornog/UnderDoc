---
id: ADR-16
type: Decision
title: How the workspace looks and works
status: erledigt
tags: [architecture, ui]
created: 2026-11-07
---

## Context

T-5 put seven questions into clickable mockups with two or three variants each. The maintainer's choice mixed variants and
added what the mockups did not show, such as tabs inside a window. A second mockup merges all of it.

## Decision

**Views**

- An icon column at the far left holds Workspace, Files, Resources and Search.
- Each entry switches the list next to it. The content area does not change: whatever a list opens lands in the same windows.

**Collections**

- A strip next to the list holds the favorite collections, stacked. It shows icons, icons with text, or text only.
- A click on a collection opens or closes it, so several can be open, one below the other. In the list, a collection is a small
  heading with a separator.
- A list of all collections has a star and a gear per row. The star decides what appears in the strip.
- Dragging things to arrange them comes later. It needs a stored place and order, so it arrives with editing.
- Folders look like documents, apart from their icon. This replaces the separators ADR-15 mentions.

**Tabs**

- Everything opens in the first window, in its general tab, which the next document replaces.
- Pinning a tab keeps it, and the next document opens in a new general tab.
- Opening a document that already has a tab in that window jumps to the tab.
- A tab can be moved to another window, duplicated and closed. A moved or duplicated tab counts as pinned.
- Tabs bring tools known from text editors: find on the page, a source view, and a status line that counts words.

**Windows**

- The default is one document in one column. There is no control for the number of windows.
- A window appears when a tab is split off, by dragging it to the right edge, with a button or from the tab's menu, up to four.
- A window disappears with its last tab.

**One document, one gear**

- What belongs to a document stays out of sight behind a gear at the document: contents, width, tags and metadata.
- Contents and width are stored per document, in its metadata. The default for contents is a marker with a drop-down, and the
  alternatives are a side panel and hidden.
- Widths are slim, wide and full. Full uses all the room the window has and hides nothing around it.
- A page view with paper formats such as DIN A4 may come later.

**Search and tags**

- The search field in the top bar stays. Search in the icon column opens a strategy: exact words, full text or similar meaning,
  and an AI answer on top if one is set up. Type and tags narrow it down.
- Every object can carry tags, and a click on a tag lists what carries it.

## Consequences

- M-14 builds the shell and the lists, M-15 the tabs and windows. M-12 and M-13 fill in the search strategies.
- Dropping the window count means the layout in the URL (M-15) lists tabs per window, not a number of columns.
- Saving contents, width and tags from the web comes with editing (M-8). Until then, they are read from the front matter, and a
  change in the view is not stored.
- Tags for what has no front matter, such as an Excel file or a database, need a place to live. That arrives with the database (M-2).
