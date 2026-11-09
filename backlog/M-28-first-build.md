---
id: M-28
type: Milestone
title: "First build: sign-in, account, collections and documents as objects"
status: offen
blocked_by: [M-1]
tags: [shell, people, architecture]
created: 2026-11-07
---

The basics of the mockup become real: the gate, an account, collections, and documents as objects with IDs (ADR-17, ADR-18). This
takes a first part of the shell from M-14, the account from M-5 and the database from M-2. Everything else that ADR-16 describes
is in sight, greyed out. The switch First build in the mockup shows what that looks like.

**Settled in the talk on 2026-11-07:**

- IDs as ADR-18 describes them, with an ID for every block from the moment it is made.
- PostgreSQL arrives now. Pages live in the database as blocks, and Markdown is the way in and out (ADR-19).
- Who sees and changes what follows ADR-20: shared collections for everyone signed in, private ones for their user, an owner per
  object, and edit, comment or read for all others.
- The phone is kept in mind roughly. Its layout is worked out later (T-10).
- Still open: what sets a collection apart from a folder. Until then, both behave alike: an object with children, a place and an
  access level.

**Done when this is true:**

- The app trusts only the signed user headers of the gate (ADR-9). Without them, nothing but the health endpoint answers.
- The first sign-in creates an account from the gate, with name, mail and groups. Administrators come from a group. The profile shows
  the account and offers signing out through the gate.
- Favorites, settings and which collections are kept open belong to the account, not to the browser.
- Collections can be made, renamed, described, starred, kept open and sorted by dragging, and a collection can sit inside another
  one and move back out (ADR-16). Deleting one that is not empty asks first.
- Pages and folders are objects with an ID, an owner, a place, children, a description and tags. They are made with New and from a
  right click on the free part of a list, moved by dragging, and deleting one that is in use asks first.
- A page is edited block by block, each in a plain Markdown field, and every version is kept. Its blocks get their IDs when they
  are made.
- A table made in a page can be pasted into another page by its address and shows there live, with a link back. Every object and
  block shows where it is used.
- Every object opens at its own address, also after a move or a rename, and links from M-1 keep working.
- Access works as ADR-20 says, and comments can be added to each element of a page where others may comment.
- The pages of the folder source from M-1 can be imported, and the whole state can be exported as Markdown and files.
- The shell parts needed for this are built as ADR-16 describes: nav1 to nav3 with their widths and stages, the Workspace view,
  Search with full text, one window with the general tab and tabs, the document head, the gear with the metadata, comments, and the
  profile with account and settings. Whatever is left for later is greyed out (ADR-17).
- The same clicks hold everywhere, as ADR-17 lists them, and a browser test with real pointer input covers them.
- At phone width, nothing is cut off and everything can be reached, even before the phone has a layout of its own.
- A backup of the database and the content store together restores the whole state, proven by a restore.
- **Acceptance: the maintainer signs in, builds a few collections with pages, nests them, embeds a table of one page in another,
  sets a collection to read only, and opens a copied link in another browser.**

**State on 2026-11-08 (frontend sprint):** the interface is the mockup itself, its look the C22 block
`.c22-app-workspace`, its data from a small JSON API. Real: the gate, the account, collections of the database (made, renamed,
nested) and of the folder source, pages opened from the folder source, full-text search, links between pages. Everything else
runs on the made-up content of the mockup, marked “(demo)”. Still to build here: pages as blocks with editing and versions,
access levels and comments, embedding by ID, settings on the account, deleting and moving, import and export, the list of
problems in the workspace, and the restore test.

**State on 2026-11-08 (pages as blocks):** pages of the database are made in a collection (New, or New page below it) and
edited block by block, each block a plain Markdown field with its ID from the start; every change is kept as a version and
listed in the gear. Still to build here: comments, embedding by ID,
deleting objects, folders in the database, import and export, the list of problems in the workspace, and the restore test.

**State on 2026-11-08 (New and dragging):** New in nav2 and in the head of a collection is one menu in groups (make, set
apart, bring in), and typing filters it. A new collection is made at once as "New collection (n)", with the first free
number, and its name is written in place. Dragging pulls the entry out of its list and opens a gap where it would land; in
nav3 moving it to the right puts it below the entry above it. Import and Link in New come later.

**State on 2026-11-09 (selecting):** Ctrl and click selects in nav2 and nav3, Shift and click a range; dragging moves the
selection, and on a window it opens as tabs. The middle third of an entry is the drop onto it; the left edge of the ghost
picks the depth. New in a collection is in blocks.

**State on 2026-11-09 (round 2):** dragging carries the entry through the list as it looks, waiting puts it below an entry,
a collection of nav2 takes it; the right click duplicates (also Ctrl at a drop) and sorts children; a single window with only
the general tab draws no tabs and has a window button beside New.

**Settled on 2026-11-09, to build:** a folder is a collection that shows only in nav3; a collection can hold collections,
which can be shown in nav2. Either turns into the other (ADR-16). Folders in the database come with this.

**State on 2026-11-09 (folders):** folders are made in the database and named in place; collections and folders turn into
each other by the right click. A folder may hold collections, and nav2 follows collections only. A collection inside another
shows in nav2 when the right click asks for it, as an entry that points at it (the trial of a box the same day is gone).

**State on 2026-11-09 (account settings):** the settings of the workspace belong to the account and follow it from browser to
browser; the browser keeps a copy.

**State on 2026-11-09 (times, private notes, trash):** moments go out in UTC and show in the time zone of the device; private
notes stay at the top; the trash empties as a whole and keeps things 30 days.

**State on 2026-11-09 (access):** the owner sets what others may do, in the overview of a collection and the settings of a
document; the database keeps it, and the lock shows in nav3.

**State on 2026-11-09 (deleting):** the right click deletes an entry of nav3 or a collection of nav2, asking in place; it
goes into a trash with everything below it, reached from the right click on the free part of nav2, which restores it where
it lay or deletes it for good. Whoever may change an object may delete it, a collection at the top only its owner; private
notes stay. Deleting asks every time, not only when something is not empty; asking when something is in use comes with
embedding.

**State on 2026-11-09 (embedding):** a block embeds a block or a whole page of another page by its address, live, with a link
back; "Copy address" on a block and pasting it alone into an empty block make the embedding, so nobody types IDs. The
database keeps every reference: a page and each of its blocks say where they are used, and a block in use asks before it
goes. Missing sources, pages the reader may not see, loops and too deep nesting show as a visible error in place. Still to
build: the list of broken references (problems in the workspace), embedding by dragging and from search while typing.

**State on 2026-11-09 (import and export):** `underdoc export` writes every collection as normal files, private notes under
`private/<login>/`, and `underdoc import` reads such a tree or a folder of the folder source into a new collection; block IDs
come back through an HTML comment before each block, and a round trip into an empty database gives the same files. The
right click on a collection downloads it as a zip. Images and other files wait for the content store, and links of the folder
source by its own IDs do not follow yet (docs/export.md).

**State on 2026-11-09 (handle, problems, links):** in reading, every block has a handle at its left with its address,
editing, comments and where it is used. The list of problems names broken embeddings and links of the pages one sees and
what the folder source ran into. Import points the links between the imported pages at their new IDs.

**State on 2026-11-09 (comments):** comments live in the database, on the whole page or on one block through its ID, with
replies one level deep; the drawer of the head, the speech bubble beside a block and its bar while editing open them, and
the head and each block count the open ones. Rights as ADR-20 says: read sees them, comment adds, edit resolves those of
others, each author changes and deletes their own. A deleted block keeps its comments as "a block that is gone". Pages of the
folder source have no comments: they are read only and have no block IDs.

**State on 2026-11-09 (tags, deleting what is used):** objects of the database carry tags, set in the gear, kept with
copies, import and export, found by the tag filter. Deleting something embedded elsewhere names those pages first.
