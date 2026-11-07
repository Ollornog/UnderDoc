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
added what the mockups did not show, such as tabs inside a window. Six more rounds merged all of it.

## Decision

**Navigation**

- Three columns, called nav1, nav2 and nav3. Every view uses all three: nav1 picks the view, nav2 the section, nav3 lists what is in it.
- The same kind of click gets the same kind of reaction everywhere. As a rule, a click in nav1 opens something in nav2, a click in
  nav2 opens something in nav3, and a click in nav3 opens into the active window. Where a third column adds nothing, nav2 opens
  straight into the window, such as an import or a new subscription.
- Everything has its own right-click menu: tabs, collections, entries in the lists, sections in nav2 and views in nav1.
- Nav1 holds the views: Workspace, Files, Resources, Bookmarks, News, modules such as Mail, and Search. Below them follow the private
  views, and at the bottom, always in sight, the profile. If room runs out, the views scroll.
- There is no bar of its own. The name, the plus for New and the search field sit above the columns, and the tabs of each window
  sit in the same row beside them. The button for the AI drawer ends that row. Without nav2 and nav3, only the plus stays there.
- There is no button to hide the navigation: a second click on the active view hides nav2 and nav3, and the next click brings them
  back.
- Whatever a list opens lands in the active window.
- Lists that belong to the user sort by dragging in nav2: to-do lists, calendars, note boards and bookmark folders. Entries that act,
  such as a new list or an import, sit apart below them.
- The right edge of each column can be dragged, and a column pushes the ones to its right. Their widths hold in every view.
- Nav1 and nav2 snap to four stages, in this order: icon, icon with text below, text, icon with text beside it. Nav3 moves freely and
  keeps its look. With this many entries, nav1 starts at the icon stage.
- Menus float over the lists. Opening one moves nothing below it.

**Collections**

- Nav2 holds the favorite collections, stacked. The Collections button at its top lists all of them in nav3, with a search, a star
  and a gear each, sorted by dragging. A second click goes back to the open collections.
- A right click on a collection, in nav2, in that list or on its heading in nav3, opens its menu: open only this, open as well,
  close, new in it, favorite, and settings. The settings open in nav3.
- By default, a click opens only that collection and Ctrl with a click opens several. A setting changes this: a click marks, so
  several stay open, and a double click opens only that one. In nav3, a collection is a small heading with a separator.
- Nav2 holds collections for now. Favorite documents join with M-16. Until accounts exist, the browser keeps the favorites.
- New with a plus at the heading of a collection adds a document, and it is dragged in the list to where it belongs. Saving that comes with editing.
- Folders look like documents, apart from their icon. This replaces the separators ADR-15 mentions.
- A click on a folder in nav3 opens it there and shows its overview, and a second click closes it. In Files, folders start closed.

**New**

- The plus in the top row opens New, and so does a double click on the empty part of a tab row.
- New always lands somewhere: in a collection chosen at its top, or in Quick notes. The last choice stays, and there are no drafts.
- Typing and Enter makes a card in Quick notes, or a Markdown page at the top of the chosen collection, from where it is dragged to
  its place. Other document types need a collection.

**Tabs**

- Every window has its own general tab, always its first tab. A list opens into the general tab of the active window, and the next
  document replaces it.
- The general tab looks different from the others, and it only exists while it holds a document. There is no empty tab.
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

- The heading is not part of the content. Title, description, when it was created and changed and by whom, and tags sit above the
  content, with the breadcrumbs and the document tools.
- Three variants are on the table. A shows the title in the breadcrumbs only, and everything else stays in the metadata. B adds one
  line with description, tags and the last change. C adds a head with title, description, type, created, changed, path and tags.
  The maintainer picks one in T-5. What the head leaves out stays in the metadata behind the gear.

**Document tools**

- All buttons for a document sit on one level below the tabs: contents, find on the page with its field right at the magnifier,
  source view, split, gear and more.
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
- Bookmarks sit in folders and can be imported from the HTML file every browser exports. Nothing is loaded from the linked sites.
- News lists entries from subscribed feeds, per user. It comes later.

**AI drawer**

- A drawer at the right edge, opened from the top bar. It knows the document in the active window and can summarize it, find related
  documents or draft a note from it. Without a set-up AI, the button does not appear.

**Profile, settings and administration**

- The profile at the bottom of nav1 holds the account and the settings in nav2, and, set apart below them, the administration.
- Settings belong to the user: theme with dark mode, page scale for the whole interface, how collections react to a click, how far lists
  open by themselves, and the column views.
- The administration holds what concerns the system, and only administrators see it: modules, users and groups, sources, storage, sign-in
  and the audit log.
- An administrator switches modules on and off there, such as mail or a document type. Off means gone for everyone, and no data is
  touched.

**Private area**

- Only its user sees it: to-dos, calendar, time clock, vacation planning and notes. Each is a view in nav1 below the others, and
  follows the same clicks: nav2 holds its lists, such as to-do lists, calendars including imported ones, periods, years or note
  boards, nav3 their entries, and an entry or the whole list opens in the active window.

## Consequences

- M-14 builds the shell, the lists and the settings, M-15 the tabs and windows. M-12 and M-13 fill in the search strategies.
- Dropping the window count means the layout in the URL (M-15) lists tabs per window, not a number of columns.
- Saving contents, width and tags from the web comes with editing (M-8). Until then, they are read from the front matter, and a
  change in the view is not stored.
- New needs editing, so it comes with M-8. Until then, the plus does not appear.
- Tags for what has no front matter, such as an Excel file or a database, need a place to live. That arrives with the database (M-2).
- Switching modules needs a module interface that can leave one out, and an administrator role. M-14 reads the switches from the
  configuration, and M-5 adds the administration.
- Network documentation is a set of document types, not a view of its own (M-23). Its IP ranges and device lists sit in the workspace.
- New modules: mail (M-22), IP addresses and devices (M-23), the private area (M-24), bookmarks (M-25), news (M-26) and further file
  sources (M-27). The AI drawer belongs to M-13.
- The user's own folder in Files needs accounts, so it comes with M-5.
- Time records and vacation plans are personal data about work, which needs its own look before M-24 is built.
