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

**State on 2026-11-09 (access):** the owner sets what others may do, in the overview of a collection and the settings of a
document; the database keeps it, and the lock shows in nav3.
