# Changelog

All notable changes to this project are documented in this file. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project uses
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed

- An entry could not be dropped onto an empty collection inside another: while the pointer waited on its middle, the held
  entry slid it away from under the pointer, and the waiting began anew. It now stays where it is while waiting.

### Changed

- Reading and editing: left of the title a large button says "Reading" or "Editing" in words and an icon and turns the
  document; where a page cannot be edited it says "Read only" and why (folder source, demo, only read or comment). The
  pencil in the head is gone. A setting under Profile, Documents, opens documents for editing (it follows the account).
- Editing a block as Docmost shows it (the idea only, no code): a bar floats above the block being written, to turn it
  into another kind (text, headings, lists, to-do, quote, code), make the selection bold, italic, struck through, code or a
  link (Ctrl+B, Ctrl+I, Ctrl+K), move the block up or down, duplicate it, copy its address or delete it. A "+" at its left
  adds a block below and opens the list of kinds, which typing "/" in an empty block opens too, filtered while typing:
  text, headings, lists, quote, code, divider, table, embed; image and file are greyed until there is a store for files.
  A block is saved when it is left. New API: `POST /api/pages/{id}/blocks/{block}/move`.
- The browser tests say why Chrome does not answer: whether it still runs, and the end of what it wrote to stderr. Once,
  on the CI, Chrome gave no answer to its first tab, and the test could not say more.
- Moments go out of the API in UTC as RFC 3339 (`…Z`) and show in the time zone of the device: when a page changed and was
  made, its versions, the trash. Before, the server wrote them in its own time, which showed UTC on the test system.
- Private notes stay at the top: they no longer move into another collection, and dragging offers them no place there.
- Dragging entries of nav2 and nav3 now comes from C22 (`app-workspace-drag.js`), as the maintainer approved it; UnderDoc
  only decides what may go where and what a drop does. A test now carries an entry somewhere else before Escape, to prove
  that Escape puts it back.
- The settings of a collection are part of its overview in the window, for the owner and whoever may edit; "Settings and
  name" opens it ready to rename, and nav3 keeps its tree instead of giving way to a settings panel.
- A right click menu stays inside the window: at the edge it moves in, and a long one scrolls. The menu of a tab and New do
  the same; before, a moved place could be overwritten right after it was set.
- The gear of the settings is Lucide's, as the drawn one read as a sun; its licence ships in `assets/app/licenses`.
- While dragging, nothing lights up under the pointer; waiting to put an entry below another takes 0.3 s instead of 0.5 s.
- Each open collection in nav3 begins with a band of its own, set apart from the one before. New keeps its scrollbar.
- ADR-16 settles what sets a folder apart from a collection: a collection can hold collections, which can be shown in nav2,
  a folder shows only in nav3; either turns into the other.
- A dragged entry moves through the list as it looks when held, instead of a picture floating over the rest. Up and down it
  always goes between entries; waiting a moment on the middle of an entry puts it below that entry, moving to the right does
  at once. An entry dropped on a collection of nav2 goes into that collection.
- A plain click is where a range starts: Shift and click right after it takes the range, Ctrl and click adds to it.
- Everything opens in the general tab, and while a single window holds nothing else, neither window nor tabs are drawn; a
  window button beside New holds what the tab offers. A second tab or window brings the tabs back.
- New in a collection puts what is needed most first: a page, then tables, databases, records, boards and lists.
- The name of an open collection in nav3 is larger; the line of a collection kept open in nav2 is slimmer, still thicker
  than one open for now; the filter of New has no box of its own.
- Ctrl and click selects entries of nav2 and nav3, Shift and click a range; dragging one moves all of them in their order, and
  dropped on a window they open there as tabs. Keeping a collection open is now a double click or the right click, and a
  middle click opens an entry as a tab of its own (both were also Ctrl and click before).
- Dragging in nav3: the middle third of an entry puts the dragged one below it, framed, while nothing moves; above and below
  that it lands before or after, and the left edge of the ghost picks the depth: level with the entry above, beside it;
  further right, below it; further left, out towards the top.
- Lifting an entry no longer makes the list jump.
- In nav2 an entry open for now draws a thin line on the left, one kept open a thick one.
- New in a collection lists the types in blocks (write, data, media, organize), each sorted by name.
- Dragging in nav2 and nav3 pulls the entry out of its list, and the others move apart where it would land, live, instead of
  a line on one edge or the other of each entry. Up and down picks the place; in nav3 moving it to the right puts it below
  the entry above it. Where one may only read, no gap opens. The private lists of nav2 sort the same way.
- New in nav2 and in the head of a collection is one menu in groups, and typing filters it; arrows and Enter pick. In nav2 it
  offers a collection, a line and a space; Import and Link come later. A new collection is made at once as
  "New collection (n)" with the first free number, and its name is written in place, in nav2 or nav3.

- nav2 tells kept open from open for now: only a collection kept open draws the line on the left, which replaces the pin.
  The favorite star is gone; nav2 keeps the order it is dragged into, with its lines and spaces. While dragging in nav2, a
  line shows where the entry lands. At the step Text, New in the head of nav2 is no longer cut off, and the first
  collection in nav3 has no line above it.

- The interface is now the approved mockup itself, with its sizes, spacing, clicks and made-up content where nothing real
  exists yet (sprint 2026-11-08). It draws itself in the browser and reads its data from a small JSON API; its look is the C22
  block `.c22-app-workspace`. Real so far: the account, the collections of the database and of the folder source with their
  trees, pages loaded when opened, full-text search on the server, making and renaming collections, and links between pages.
  The made-up collections of the mockup are marked “(demo)”. The browser keeps widths, favorites, pinned collections and the
  theme until the account stores them.
- `/dev/ui` shows the workspace with only the content of the mockup.
- The interface is the workspace of ADR-16, first part (M-28): a top row with name, search and New, nav1 with the views, nav2
  with the collections, nav3 with the open collection, and the window with its head. What is not built yet stays in sight,
  greyed out. A click in nav3 opens the object in the window without reloading the list, and a double click or the arrow shows
  or hides what is in it. The right edges of the columns can be dragged, nav1 and nav2 snap to four steps, and the browser keeps
  the widths and the theme.
- The look follows the approved mockup: compact headings and spacing in pages, tables framed with a shaded head, the way
  above the title starting at the collection, small tags, a larger title, all tools of the window (those not built yet greyed
  out), a status line with words, characters and reading time, the list of tabs at the end of the tab row, and the entries
  of a collection or folder as framed rows with their kind.
- The workspace is now the C22 building block (C22 T-13): the structure, the column steps, the lists, the drawer, the menus
  and the right-click menu of an entry come from C22, and UnderDoc keeps only its own sizes from the mockup. The stored
  column widths start over once.
- The profile is a view like the others: its sections in nav2, the chosen one in nav3, and the administration set apart for
  administrators. A click in nav1 or nav2 changes only the columns, the window keeps what it shows (ADR-16).
- Every object opens at `/o/<id>` (ADR-18). Links to `/d/<id>` move there.
- The title stands in the head of the window with the breadcrumbs, the tags and the description, not in the content.
- The CSP allows requests to the own origin (`connect-src 'self'`), so the window can be swapped in.
- `scripts/vendor-c22.sh` takes the licence text of the Inter font from C22 instead of keeping its own copy current by hand.

### Added

- Embedding by ID (ADR-18): a block line `::embed{ref="<page id>#<block id>"}`, or without `#<block id>` for a whole
  page, shows that source live, framed, with a link back. "Copy address" in the bar of a block puts its address on the
  clipboard; pasted alone into an empty block, it becomes the embedding and is saved. The database keeps every reference
  (migration 006, table `refs`), so a page lists where it and its blocks are used, and a block in editing shows "Used in n".
  A block in use asks before it is deleted, also when empty. What cannot be shown says why: a missing page or block, a page
  the reader may not see, a loop, or embedding deeper than five levels. Copies of a page embed what the original did.
- Export and import (ADR-13, ADR-19, [docs/export.md](docs/export.md)): `underdoc export <dir>` writes every collection as
  normal files, collections and folders as directories with a small `.underdoc.yaml` for ID, kind, title, access and the
  order with lines and spaces, pages as Markdown with front matter and an HTML comment `<!-- block bN -->` before each
  block, so the block IDs come back without showing in any viewer; private notes of every account go to
  `private/<login>/`. `underdoc import --owner <login> <dir> <title>` reads such a tree, or any folder of Markdown files,
  into a new collection: IDs are kept where valid and free, nothing that exists is changed, and a page without markers is
  split at its headings. Export and import into an empty database give the same files again. The right click menu of a
  collection offers "Export as files", a zip of what the viewer sees (`GET /api/objects/{id}/export`).
- The trash empties as a whole ("Empty the trash", asking in place; `DELETE /api/trash`), and what lies there longer than
  30 days goes for good by itself, checked at start and every hour.
- What others may do is kept in the database (`POST /api/objects/{id}/access`, part of the tree in `/api/state`): the owner
  sets it in the overview of a collection or the settings of a document, below the top also "as above"; others see it
  greyed out. Before, it lasted only until a reload, and the lock of a collection of the database never showed.
- The settings of the workspace belong to the account (`accounts.settings`, migration 004, `PUT /api/settings`, part of
  `/api/state`): the order of nav2 with its lines, spaces and shown collections, what is kept open, widths, zoom and theme
  follow it from browser to browser. A change goes to the server a moment later; the browser keeps a copy, and without the
  database only that. At most 64 KiB, one JSON object.
- Deleting with a trash: the right click on an entry of nav3 or a collection of nav2 ends with Delete, which asks in place
  ("Delete? Yes") before it acts. What lies in the database goes into the trash with everything below it (migration
  `005_trash.sql`, `DELETE /api/objects/{id}`), leaves every list, and windows showing it let go of it; made-up content is
  deleted in the browser only. Whoever may change an object may delete it, a collection at the top only its owner, and
  private notes are never deleted. Lines and spaces still go for good.
- The trash, from the right click on the free part of nav2: a page in the window listing what one deleted or owns, newest
  first, with where it lay, when and by whom (`GET /api/trash`). Restore puts it back where it lay, at the end, with what
  was deleted with it (`POST /api/trash/{id}/restore`); what lay in something that is in the trash too waits until that is
  restored. Delete for good asks in place as well and takes blocks and versions with it (`DELETE /api/trash/{id}`).
- Folders in the database: New in a collection makes one, named in place (`POST /api/folders`), and the right click turns
  a collection into a folder or back (`POST /api/objects/{id}/kind`). A folder shows only in nav3; a collection at the top
  stays one. Collections of the database now have the icon of a collection, not of a folder.
- A collection inside another can be shown in nav2 as well, by the right click ("Show in nav2"); the entry only points at it,
  opens it in nav3 as a heading of its own, and goes with "Remove from nav2". The browser keeps it with the order of nav2.
  This replaces a trial of the same day that showed the children of a collection in nav2 inside its box.
- Duplicating: the right click copies an entry with everything below it, right after it, and Ctrl held at a drop copies
  instead of moving (`POST /api/objects/{id}/copy`, at most 500 objects at once). The copies belong to whoever copies, pages
  keep their blocks and block IDs, and each copy starts its own history.
- Sorting: the right click on anything holding two or more entries sorts its children A to Z or Z to A; lines and spaces
  stay where they are.
- Every entry of a right click menu carries an icon.
- Lines and spaces set entries apart, in nav2 and in nav3: right click on an entry (Line below it, Space below it), on a
  collection head in nav3 (at the top of it) or on the free part of nav2. In nav2 they are the user's own and the browser
  keeps them with the order; in a collection of the database they are objects, shared with everyone who sees it
  (`POST /api/dividers`, `DELETE /api/objects/<id>` for now only for them).
- Dragging an object of the database in nav3 moves it for good (`POST /api/objects/<id>/move`): only where one may edit,
  nothing into itself or below itself, only a collection at the top, and only its owner takes an object into a private
  collection. If the server says no, the list goes back.
- Backlog M-29: recipes as a document type.
- Pages in the database, edited block by block (ADR-19, M-28): New in a collection, or New page below it, makes a page that
  opens with its first block ready to write. Edit in the window turns every block into a plain Markdown field, saved when it
  loses the focus or on Ctrl+Enter; Escape takes a change back. A block gets its ID when it is made (`b1`, `b2` …, never
  given out twice in a page), and a table shows the ID of its block. Every change is kept as a version with who and when,
  listed in the gear. Deleting a block with text asks first, in place. Others may change a page only where they may edit
  (ADR-20). API: `POST /api/pages`, `POST /api/pages/<id>/blocks`, `PUT` and `DELETE /api/pages/<id>/blocks/<block>`,
  `GET /api/pages/<id>/versions`. The look of the fields comes from C22 (`.bedit`, `.bbar`).
- A JSON API for the workspace: `/api/state` (account and collections with their trees), `/api/doc/<id>`, `/api/search`,
  `POST /api/collections` and `PATCH /api/objects/<id>` (rename, for the owner or where others may edit, ADR-20). Changes are
  taken only from this application's own pages.
- PostgreSQL, optional through `UNDERDOC_DATABASE_URL` (ADR-19): accounts from the gate, each with a private collection My
  notes, and collections made with New in nav2 or inside another one from its heading in nav3, with IDs of ADR-18. A
  collection opens its page with owner, what others may do and who sees it. Changes are taken only from this
  application's own pages. The tests start a throwaway PostgreSQL and fail without one.
- The user comes from the headers of the sign-in gate, and without them nothing but the health endpoint answers. A profile
  shows the account, signing out, the theme and, to administrators, the administration still to come.
- `description` and `tags` in the front matter. A click on a tag lists everything that carries it.
- Project skeleton: concept, backlog with milestones and decisions, CI, repository hygiene.
- Plan for versions and audit (ADR-11).
- Logo and favicon.
- Plans for search (M-12) and AI (M-13).
- Plans for the workspace with tabs and windows, favorites and inbox, import and export, calendar, forms, reviews through Git
  and a site builder (M-14 to M-21), and an order of work: one topic at a time, built as modules (ADR-14).
- Plan for a workspace of typed documents with three views: workspace, files and resources (ADR-15).
- How the workspace looks and works: icon column, favorite collections, tabs, windows that come and go, a gear per document, tags and
  search strategies (ADR-16).
- Plans for more document types (to-do list, kanban board, video, PowerPoint, quick notes), settings with modules an administrator
  switches on and off, mail (M-22), IP addresses and devices (M-23), and a private area (M-24).
- Plans for bookmarks (M-25), news from feeds (M-26) and more file sources (M-27). Every window gets its own general tab with a
  history, tabs behave as in code editors, New lands in a chosen collection or in Quick notes, one rule for clicks holds in every
  view, settings move into the profile, and an AI drawer sits at the right edge. The tabs share the top row, the heading moves out
  of the content, and saved searches become a document type (ADR-16).
- Guiding principles for every feature (ADR-17) and a proposal for object IDs and addresses (ADR-18). Every collection sits in nav2,
  a collection is an object that can sit inside another and can be kept open, Ctrl with a click adds without replacing, and the document head keeps the
  title below the breadcrumbs (ADR-16). A draft for the first build with sign-in, account, collections and documents as objects
  (M-28), and mockups for the phone layout (T-10).
- Pages live in the database as blocks with IDs, and Markdown is the way in and out (ADR-19). Who sees and changes what before
  roles arrive: an owner per object, and edit, comment or read for all others (ADR-20). The scope of the first build is settled
  (M-28): PostgreSQL now, comments, tables embedded by their ID, New collection in nav2 and from a right click on free space, and
  private lists that open like collections (ADR-16).
- A click in a list opens the object, also a collection with its page, and a double click or the arrow opens its children. New
  collection sits at the top of nav2, the plus moves to the right of the search field, bookmarks become a document type instead of
  a view, and comments belong to an element of a page (ADR-16, ADR-17, ADR-20, M-25).

### Removed

- The pages rendered on the server, among them `/search` and `/problems`, and their scripts: one interface instead of two
  (ADR-17). The number of problems is in `/api/state`; the list of them comes back in the workspace.

### Security

- A collection could be made inside another one where others may only comment or read. Now that needs the right to edit
  there, as everything else does (ADR-20).
- The Markdown of a block stays text in its field, and its view drops raw HTML like every page; a browser test proves both.
- Names, descriptions, tags and paths from files and from people are shown as text everywhere in the workspace; a browser
  test proves it with names full of markup.

### Added

- A folder of Markdown files appears as a wiki: tree, pages, `[[wiki links]]`, backlinks, table of contents, images next to a
  page, full-text search, recent changes and a list of everything broken (M-1).
- Pages with an `id:` in their front matter keep their address and their links when the file moves.
- Changes on disk appear within seconds, without a restart.
- One binary or one container, read-only access to the folder, a strict CSP and a health endpoint.

### Fixed

- A link to a folder opens that folder instead of counting as broken (B-1).

### Changed

- Documents have their own identity and roles, separate from the folder tree (ADR-13, replaces ADR-3).
- The first milestone is now a Markdown wiki only. Excel, Samba and versions moved to M-2 (ADR-12).
