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
added what the mockups did not show, such as tabs inside a window. Three more rounds merged all of it.

## Decision

**Navigation**

- Three columns, called nav1, nav2 and nav3. Every view uses all three: nav1 picks the view, nav2 the section, nav3 lists what is in it.
- Nav1 holds the views: Workspace, Private, Files, Resources, modules such as Mail, and Search. At the bottom, set apart, are Settings
  for the user and Admin for the system. The profile sits at the top right.
- Whatever a list opens lands in the same windows. A second click on the active view hides nav2 and nav3.
- The right edge of each column can be dragged, and a column pushes the ones to its right. Their widths hold in every view.
- Nav1 and nav2 snap to four stages, in this order: icon, icon with text below, text, icon with text beside it. Nav3 moves freely and
  keeps its look.

**Collections**

- Nav2 holds the favorite collections, stacked. The Collections button at its top shows all of them in nav2, with search, and a star
  and a gear as far as the stage has room. The gear opens the collection's settings in nav3.
- By default, a click opens only that collection and Ctrl with a click opens several. A setting changes this: a click marks, so
  several stay open, and a double click opens only that one. In nav3, a collection is a small heading with a separator.
- Nav2 holds collections for now. Favorite documents join with M-16. Until accounts exist, the browser keeps the favorites.
- A plus at a collection adds a document, and it is dragged in the list to where it belongs. Saving that comes with editing.
- Folders look like documents, apart from their icon. This replaces the separators ADR-15 mentions.

**Tabs**

- Every link and list opens in the general tab, which the next document replaces.
- The general tab is always the first tab of the leftmost window. It stays there, looks different from the others and cannot be dragged.
- An icon in the general tab, the usual one for opening something new, opens its document as a tab of its own at once, and the general
  tab stands ready for the next document. No other tab has a pin.
- Opening a document that already has a tab in that window jumps to the tab.
- Dragging sorts tabs and moves them to another window, and a dragged tab snaps: it takes its place in the row, and the others make
  room. A tab can also be duplicated and closed.
- Tabs shrink like browser tabs, so none hides beyond the edge of a narrow window.

**Document tools**

- All buttons for a document sit on one level below the tabs: contents, find on the page with its field right at the magnifier,
  source view, split, gear and more.
- A status line counts words.

**Windows**

- The default is one document in one column. There is no control for the number of windows, and windows have no numbers.
- A window appears when a tab is split off, up to four. Dragging a tab to the left or right third of a window opens a column on that
  side, and a preview shows the result first. A button and the menu do the same.
- A column on the far left takes the general tab along. Its document becomes a normal tab in the window it was in.
- The active window has a clear frame, and the others step back. A window disappears with its last tab, except the leftmost one.

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

- There is one search field, in the top bar, and typing shows hits at once in nav3. Search in nav1 opens the same place and puts the
  cursor in that field.
- Nav2 picks the strategy: exact words, full text or similar meaning, and an AI answer on top if one is set up. Filters by type and
  tags stay behind a button. Hits come with the matching text, and Enter opens the first.
- Every object can carry tags, and a click on a tag lists what carries it.

**Settings and admin**

- Settings belong to the user: theme with dark mode, page scale for the whole interface, how collections react to a click, how far lists
  open by themselves, and the column views.
- The admin panel holds what concerns the system, and only administrators see it: modules, users and groups, sources, storage, sign-in
  and the audit log.
- An administrator switches modules on and off there, such as mail or a document type. Off means gone for everyone, and no data is
  touched.

**Private area**

- A view only its user can see, for their own to-dos, calendar, time clock, vacation planning and notes.

## Consequences

- M-14 builds the shell, the lists and the settings, M-15 the tabs and windows. M-12 and M-13 fill in the search strategies.
- Dropping the window count means the layout in the URL (M-15) lists tabs per window, not a number of columns.
- Saving contents, width and tags from the web comes with editing (M-8). Until then, they are read from the front matter, and a
  change in the view is not stored.
- Tags for what has no front matter, such as an Excel file or a database, need a place to live. That arrives with the database (M-2).
- Switching modules needs a module interface that can leave one out, and an administrator role. M-14 reads the switches from the
  configuration, and M-5 adds the panel.
- Network documentation is a set of document types, not a view of its own (M-23). Its IP ranges and device lists sit in the workspace.
- New modules: mail (M-22), IP addresses and devices (M-23), and the private area (M-24).
- Time records and vacation plans are personal data about work, which needs its own look before M-24 is built.
