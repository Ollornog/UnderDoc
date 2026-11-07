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
added what the mockups did not show, such as tabs inside a window. Four more rounds merged all of it.

## Decision

**Navigation**

- Three columns, called nav1, nav2 and nav3. Every view uses all three: nav1 picks the view, nav2 the section, nav3 lists what is in it.
- Nav1 holds the views: Workspace, Files, Resources, Bookmarks, News, modules such as Mail, and Search. At the bottom follow the
  private entries, then, always in sight, the profile, Settings for the user and Admin for the system. If room runs out, the views
  scroll.
- The top bar holds a plus for New, the search field and a button for the AI drawer.
- Whatever a list opens lands in the active window. A second click on the active view hides nav2 and nav3.
- The right edge of each column can be dragged, and a column pushes the ones to its right. Their widths hold in every view.
- Nav1 and nav2 snap to four stages, in this order: icon, icon with text below, text, icon with text beside it. Nav3 moves freely and
  keeps its look. With this many entries, nav1 starts at the icon stage.
- Menus float over the lists. Opening one moves nothing below it.

**Collections**

- Nav2 holds the favorite collections, stacked. The Collections button at its top shows all of them in nav2, and its search field
  takes the place of the button, so the list does not move. In the narrowest stage the field floats next to it.
- Every collection has a menu, behind three dots on hover or a right click, at every width: open only this, open as well, favorite,
  and settings. The settings open in nav3.
- By default, a click opens only that collection and Ctrl with a click opens several. A setting changes this: a click marks, so
  several stay open, and a double click opens only that one. In nav3, a collection is a small heading with a separator.
- Nav2 holds collections for now. Favorite documents join with M-16. Until accounts exist, the browser keeps the favorites.
- A plus at a collection adds a document, and it is dragged in the list to where it belongs. Saving that comes with editing.
- Folders look like documents, apart from their icon. This replaces the separators ADR-15 mentions.

**New and drafts**

- The plus in the top bar opens New, and so does a click on an empty general tab. Typing and Enter makes a Markdown draft at once,
  and picking a type makes an empty draft of that type.
- A draft needs no place. Drafts sit in nav2 of the Workspace until they are placed, so a quick note needs no thought about
  structure.
- A draft shows where it can go and who sees it: private, a group or public. Rights beyond that come with M-5.

**Tabs**

- Every window has its own general tab, always its first tab. A list opens into the general tab of the active window, and the next
  document replaces it.
- The general tab looks different from the others. Empty, it is an empty tab and carries no icon.
- Dragging the general tab keeps its document as a tab of its own where it lands, and the general tab stays, empty. The tab menu
  does the same.
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
- A window appears when a tab is split off, up to four, and it brings its own general tab. Dragging a tab to the left or right third
  of a window opens a column on that side, and a preview shows the result first. A button and the menu do the same.
- The active window has a line along its top, and the others step back a little.
- A window disappears when only its empty general tab is left, unless it is the last one.

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

**Files, bookmarks and news**

- Files lists its roots in nav2: the user's own folder, the share, and later further sources a user adds, such as an SMB share,
  WebDAV, SFTP or S3.
- Bookmarks sit in folders and can be imported from the HTML file every browser exports. Nothing is loaded from the linked sites.
- News lists entries from subscribed feeds, per user. It comes later.

**AI drawer**

- A drawer at the right edge, opened from the top bar. It knows the document in the active window and can summarize it, find related
  documents or draft a note from it. Without a set-up AI, the button does not appear.

**Settings and admin**

- Settings belong to the user: theme with dark mode, page scale for the whole interface, how collections react to a click, how far lists
  open by themselves, and the column views.
- The admin panel holds what concerns the system, and only administrators see it: modules, users and groups, sources, storage, sign-in
  and the audit log.
- An administrator switches modules on and off there, such as mail or a document type. Off means gone for everyone, and no data is
  touched.

**Private area**

- Only its user sees it: to-dos, calendar, time clock, vacation planning and notes. Its entries sit at the bottom of nav1 and open
  straight into the active window, without a view of their own.

## Consequences

- M-14 builds the shell, the lists and the settings, M-15 the tabs and windows. M-12 and M-13 fill in the search strategies.
- Dropping the window count means the layout in the URL (M-15) lists tabs per window, not a number of columns.
- Saving contents, width and tags from the web comes with editing (M-8). Until then, they are read from the front matter, and a
  change in the view is not stored.
- New and drafts need editing, so they come with M-8. Until then, the plus does not appear. Who may see a draft needs M-5.
- Tags for what has no front matter, such as an Excel file or a database, need a place to live. That arrives with the database (M-2).
- Switching modules needs a module interface that can leave one out, and an administrator role. M-14 reads the switches from the
  configuration, and M-5 adds the panel.
- Network documentation is a set of document types, not a view of its own (M-23). Its IP ranges and device lists sit in the workspace.
- New modules: mail (M-22), IP addresses and devices (M-23), the private area (M-24), bookmarks (M-25), news (M-26) and further file
  sources (M-27). The AI drawer belongs to M-13.
- The user's own folder in Files needs accounts, so it comes with M-5.
- Time records and vacation plans are personal data about work, which needs its own look before M-24 is built.
