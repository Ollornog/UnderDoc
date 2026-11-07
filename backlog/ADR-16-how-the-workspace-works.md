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
added what the mockups did not show, such as tabs inside a window. Two more rounds merged all of it.

## Decision

**Navigation**

- Three columns, called nav1, nav2 and nav3. Nav1 is the icon column at the far left. Nav2 and nav3 together are the navigation.
- Nav1 holds the views: Workspace, Files, Resources, modules such as Mail and Network, and Search. At the bottom, set apart, are
  Profile and Settings.
- Each entry switches the navigation next to it. The content area does not change: whatever a list opens lands in the same windows.
- The navigation has one width in every view, and dragging its edge changes it. A second click on the active view hides it.

**Collections**

- Nav2 holds the favorite collections, stacked, as icons, icons with text, or text only. That choice is a setting.
- A click on a collection opens or closes it, so several can be open, one below the other. In nav3, a collection is a small
  heading with a separator.
- A Collections button at the top of nav2 widens nav2 over nav3 and shows all collections, to search and edit them, with a star
  and a gear per row. The star decides what appears in nav2.
- Nav2 holds collections for now. Favorite documents join with M-16. Until accounts exist, the browser keeps the favorites.
- Dragging things to arrange them comes later. It needs a stored place and order, so it arrives with editing.
- Folders look like documents, apart from their icon. This replaces the separators ADR-15 mentions.

**Tabs**

- Every link and list opens in the main window, in its general tab, which the next document replaces.
- The general tab is always the first tab, looks different from the others, and cannot be moved away.
- A button keeps the document as a tab of its own at once, and the general tab stands ready for the next document. No tab has a
  pin icon.
- Opening a document that already has a tab in that window jumps to the tab.
- Dragging sorts tabs and moves them to another window. A tab can also be duplicated and closed.
- Tabs bring tools known from text editors: find on the page, with its field right at the magnifier, a source view, and a status
  line that counts words.

**Windows**

- The default is one document in one column. There is no control for the number of windows.
- A window appears when a tab is split off, up to four. Dragging a tab to the right edge opens a column on the right, and to the
  left edge one on the left. A button and the tab's menu do the same.
- A window disappears with its last tab. The main window stays, as it holds the general tab.

**One document, one gear**

- What belongs to a document stays out of sight behind a gear at the document: contents, width, tags and metadata. It opens as a
  drawer over its window, so it also works in a narrow one.
- Contents and width are stored per document, in its metadata. The default for contents is a marker with a drop-down, and the
  alternatives are a side panel and hidden.
- Widths are slim, wide and full. Full uses all the room the window has and hides nothing around it.
- Content starts at the left, next to the navigation, with a fitting margin, whatever its width. It is not centered in the window.
  Inside a document, blocks can still be aligned left, center or right.
- A page view with paper formats such as DIN A4 may come later.

**Search and tags**

- The search field in the top bar stays. Search in nav1 opens a strategy: exact words, full text or similar meaning, and an AI
  answer on top if one is set up. Type and tags narrow it down.
- Every object can carry tags, and a click on a tag lists what carries it.

**Settings and profile**

- Settings are separate from the profile. They hold the theme with dark mode, the page scale, how favorites show, and how far lists
  open by themselves.
- An administrator switches modules on and off there, such as mail or a document type. Off means gone for everyone, and no data is
  touched.

## Consequences

- M-14 builds the shell, the lists and the settings, M-15 the tabs and windows. M-12 and M-13 fill in the search strategies.
- Dropping the window count means the layout in the URL (M-15) lists tabs per window, not a number of columns.
- Saving contents, width and tags from the web comes with editing (M-8). Until then, they are read from the front matter, and a
  change in the view is not stored.
- Tags for what has no front matter, such as an Excel file or a database, need a place to live. That arrives with the database (M-2).
- Switching modules needs a module interface that can leave one out, and an administrator role. M-14 reads the switches from the
  configuration, and M-5 adds the screen.
- New modules from this round: mail (M-22) and IP addresses and devices (M-23).
