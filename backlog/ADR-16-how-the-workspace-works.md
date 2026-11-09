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
added what the mockups did not show, such as tabs inside a window. Seven more rounds merged all of it. The rules below are worked
examples of the principles in [ADR-17](ADR-17-guiding-principles.md).

## Decision

**Navigation**

- Three columns, called nav1, nav2 and nav3. Every view uses all three: nav1 picks the view, nav2 the section, nav3 lists what is in it.
- The same kind of click gets the same kind of reaction everywhere. As a rule, a click in nav1 opens something in nav2, a click in
  nav2 opens something in nav3, and a click in nav3 opens into the active window. Where a third column adds nothing, nav2 opens
  straight into the window, such as an import or a new subscription.
- Everything has its own right-click menu: tabs, collections, entries in the lists, sections in nav2 and views in nav1.
- Nav1 holds the views: Workspace, Files, Resources, News, modules such as Mail, and Search. Below them follow the private views, and at
  the bottom, always in sight, the profile. If room runs out, the views scroll.
- There is no bar of its own. The name, the search field and the plus for New sit above the columns, in that order, and the tabs of
  each window sit in the same row beside them. The button for the AI drawer ends that row. Without nav2 and nav3, only the plus stays
  there.
- There is no button to hide the navigation: a second click on the active view hides nav2 and nav3, and the next click brings them
  back.
- Whatever a list opens lands in the active window.
- Lists that belong to the user sort by dragging in nav2: to-do lists, calendars and note boards. Entries that act,
  such as a new list or an import, sit apart below them.
- The right edge of each column can be dragged, and a column pushes the ones to its right. Their widths hold in every view.
- Nav1 and nav2 snap to four stages, in this order: icon, icon with text below, text, icon with text beside it. Nav3 moves freely and
  keeps its look. With this many entries, nav1 starts at the icon stage.
- Menus float over the lists. Opening one moves nothing below it.

**Collections**

- Nav2 holds every collection, so an open one is always in sight: the favorites on top, the others below a line. They sort by
  dragging, and a star at each one marks a favorite.
- A collection is an object like a document (ADR-17). It opens its overview in the window, and it can sit inside another collection,
  where it shows in nav3 like a document with children. Dragging it from nav2 into nav3 puts it inside another, and dragging it
  back onto nav2 makes it stand on its own again.
- **Collection and folder (the maintainer, 2026-11-09):** the two are the same but for one thing: a collection can hold
  collections, and those can be shown in nav2; a folder shows only in nav3. Either can be turned into the other. Until
  2026-11-09 this was open, and a collection inside a collection was treated much like a folder (M-28).
  - A folder may hold collections too. nav2 follows collections only and leaves everything else aside: a collection inside a
    folder shows in nav3 alone. A collection at the top stays one, as only collections stand there.
  - **Shown in nav2 (2026-11-09):** a collection inside another lies in nav3 only, until the right click shows it in nav2
    as well. That entry only points at it: the collection stays where it lies, opens in nav3 as a heading of its own, and the
    entry goes with "Remove from nav2" or when it stops being a collection. Until accounts store it, the browser keeps it
    with the order of nav2. ~~A trial the same day showed the children of a collection in nav2 inside its box~~; the
    maintainer did not like it.
- **Menus stay inside the window (2026-11-09):** a right click menu at the edge moves in, and a long one scrolls.
- New collection sits at the top of nav2, as New + sits at the heading of a collection in nav3. A right click on the free part of nav2
  offers it too, a right click on the free part of nav3 offers New in each open collection and New collection, and the menu of a
  collection offers a new collection inside it. A new collection opens with its settings, ready to be named.
- A right click on a collection, in nav2 or on its heading in nav3, opens its menu: open, keep open or stop, close, its overview,
  new in it, favorite, and settings. The settings open in nav3.
- A collection can be kept open. A click opens a collection and closes every open one that is not kept open. A double click, or
  the right click, keeps it open, or stops and closes it; until 2026-11-09 Ctrl with a click did too, which now selects. In nav2 a
  thick line on the left marks what is kept open and a thin one what is open for now (2026-11-09, before that a pin), and
  closing it at its heading stops keeping it open. There is no setting for this. In nav3, an open collection is a small heading
  with a separator.
- Favorite documents join nav2 with M-16. Until accounts exist, the browser keeps the favorites.
- New with a plus at the heading of a collection adds a document, and it is dragged in the list to where it belongs. Saving that comes with editing.
- Folders look like documents, apart from their icon. This replaces the separators ADR-15 mentions.
- A click on an entry in nav3 opens the object, whatever it is: a document, a folder with its overview, or a collection with its
  page. That page shows the name and description in the head, and the owner, what others may do, who sees it and its entries. A click
  on the heading of an open collection opens that page too, it does not close the collection. A double click or the arrow opens or
  closes the children. In Files, folders start closed.
- A middle click on an entry in nav3 opens it as a tab of its own, and the general tab stays as it is; until 2026-11-09 Ctrl
  with a click did too.
- **Selecting (2026-11-09):** Ctrl with a click adds an entry of nav2 or nav3 to a selection or takes it out, Shift with a click
  takes the range from the last one; a selection stays in one list, and a plain click or Escape ends it. Dragging one of them
  moves them all, in their order. Dropped on a window, entries open there as tabs; the right click on a selection opens them
  as tabs (nav3), opens or keeps them open (nav2). A plain click is where a range starts, so Shift with a click right after
  it takes the range (2026-11-09, round 2).
- **Dragging (2026-11-09, round 2):** a dragged entry moves through the list as it looks when held, nothing floats over the
  rest. Up and down it goes between entries; waiting a moment on the middle of an entry puts it below that entry, and moving
  to the right does at once, moving to the left takes it out towards the top. On a collection of nav2 it goes into that
  collection. With Ctrl held at the drop it is copied.
- **Duplicate and sort (2026-11-09):** the right click duplicates an entry (with everything below it, as a copy of the one
  who copies) and sorts the children of anything holding two or more, A to Z or Z to A; lines and spaces stay where they
  are, and each group between them is sorted on its own. Every entry of a menu carries its icon.

**New**

- The plus in the top row opens New, and so does a double click on the empty part of a tab row.
- New always lands somewhere: in a collection chosen at its top, or in Quick notes. The last choice stays, and there are no drafts.
- Typing and Enter makes a card in Quick notes, or a Markdown page at the top of the chosen collection, from where it is dragged to
  its place. Other document types need a collection.

**Tabs**

- Every window has its own general tab, always its first tab. A list opens into the general tab of the active window, and the next
  document replaces it.
- The general tab looks different from the others, and it only exists while it holds a document. There is no empty tab.
- **One window, one general tab (2026-11-09):** everything opens in the general tab. While a single window holds nothing else,
  neither the window nor its tabs are drawn: the page stands alone, and a window button beside New holds what the tab and its
  arrow offer. A second tab or a second window brings the tabs back.
- One click on it selects it, like any tab. What was opened in it before lies below it, drawn as a stack of tabs, and its arrow lists
  those, up to 15, to open one again. The arrow means history, the right click means actions.
- When the general tab gives up its document, by closing it or keeping it as a tab of its own, the one opened before takes its
  place. Dragging the general tab keeps its document as a tab where it lands. The tab menu does the same.
- Opening a document that already has a tab in that window jumps to the tab.
- Dragging sorts tabs and moves them to another window, and a dragged tab snaps: it takes its place in the row, and the others make
  room.
- Tabs behave as in code editors. A right click on a tab and the dots below the tabs show the same menu: close, close others, close
  to the right or left, close all, duplicate or keep, copy path, move to a new column or another window, and close the window. A
  middle click closes a tab. A button at the end of the row lists all tabs of the window.
- Tabs shrink like browser tabs, so none hides beyond the edge of a narrow window.

**Head of a document**

- The heading is not part of the content. It sits in the same bar as the breadcrumbs and the document tools: the breadcrumbs first,
  the tags to their right, and below them the title in large type with the description under it.
- Created, changed, by whom, type and path are metadata behind the gear, not part of the head. Of the three variants in T-5, this
  keeps the large title of the third.

**Document tools**

- All buttons for a document sit on one level below the tabs: contents, find on the page with its field right at the magnifier,
  source view, split, comments, gear and more.
- Comments belong to an element of the page: a paragraph, heading, list or table, through its ID (ADR-18). Pointing at an element
  shows a speech bubble beside it, which carries the number of its comments and opens them in a drawer over the window, like the
  gear. The comments button in the tools lists every commented element of the page.
- Every table in a page shows a small button with its ID, which copies its address. Pasted into another page, the table shows there
  live, framed and with a link to its source (ADR-18).
- A menu with no room below the button that opens it opens above it.
- A status line counts words.

**Windows**

- The default is one document in one column. There is no control for the number of windows, and windows have no numbers.
- A window appears when a tab is split off, up to four, and it brings its own general tab. Dragging a tab to the left or right third
  of a window opens a column on that side, and a preview shows the result first. A button and the menu do the same.
- The active window has a line along its top, and the others step back a little.
- A window disappears when it has no tab left, unless it is the last one. With more than one window, each has a
  button to close it with all its tabs.

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
- A saved search is a document type: a query with strategy, filters and sources, placed in a collection like any document. It runs
  again each time it opens.

**Files, bookmarks and news**

- Files lists its roots in nav2: the user's own folder, the share, and later further sources a user adds, such as an SMB share,
  WebDAV, SFTP or S3.
- Bookmarks are not a view of their own but objects: a bookmark is a document type, placed in folders and collections like any other.
  New at a collection imports the HTML file every browser exports. Nothing is loaded from the linked sites.
- News lists entries from subscribed feeds, per user. It comes later.

**AI drawer**

- A drawer at the right edge, opened from the top bar. It knows the document in the active window and can summarize it, find related
  documents or draft a note from it. Without a set-up AI, the button does not appear.

**Profile, settings and administration**

- The profile at the bottom of nav1 holds the account and the settings in nav2, and, set apart below them, the administration.
- Settings belong to the user: theme with dark mode, page scale for the whole interface, how far lists open by themselves, and the
  column views.
- The administration holds what concerns the system, and only administrators see it: modules, users and groups, sources, storage, sign-in
  and the audit log.
- An administrator switches modules on and off there, such as mail or a document type. Off means gone for everyone, and no data is
  touched.

**Private area**

- Only its user sees it: to-dos, calendar, time clock, vacation planning and notes. Each is a view in nav1 below the others, and
  follows the same clicks: nav2 holds its lists, such as to-do lists, calendars including imported ones, periods, years or note
  boards, nav3 their entries, and an entry or the whole list opens in the active window.
- Its lists open like collections: a click opens one and closes the others that are not kept open, and a double click keeps
  one open, marked like a collection; Ctrl with a click selects (2026-11-09). Open lists stack in nav3 below small headings.

**Access**

- What others may do shows where it matters, as ADR-20 sets it: a lock at the heading of a collection that others may only read or
  comment, and a note next to the tags of such a document. What the level forbids is greyed out, such as New, New page below it or
  dragging.
- The settings of a collection and the gear of a document set the level: edit, comment or read, or inherited from above.

## Consequences

- M-14 builds the shell, the lists and the settings, M-15 the tabs and windows. M-12 and M-13 fill in the search strategies.
- Dropping the window count means the layout in the URL (M-15) lists tabs per window, not a number of columns.
- Saving contents, width and tags from the web comes with editing (M-8). Until then, they are read from the front matter, and a
  change in the view is not stored.
- New needs somewhere to store what it makes, so it comes with M-28 or M-8. Until then, the plus is greyed out (ADR-17).
- The first build (M-28) builds a part of this, and the rest stays in sight, greyed out.
- The phone gets its own layout, settled in T-10.
- Tags for what has no front matter, such as an Excel file or a database, need a place to live. That arrives with the database (M-2).
- Switching modules needs a module interface that can leave one out, and an administrator role. M-14 reads the switches from the
  configuration, and M-5 adds the administration.
- Network documentation is a set of document types, not a view of its own (M-23). Its IP ranges and device lists sit in the workspace.
- New modules: mail (M-22), IP addresses and devices (M-23), the private area (M-24), bookmarks (M-25), news (M-26) and further file
  sources (M-27). The AI drawer belongs to M-13.
- The user's own folder in Files needs accounts, so it comes with M-5.
- Time records and vacation plans are personal data about work, which needs its own look before M-24 is built.
