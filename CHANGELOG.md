# Changelog

All notable changes to this project are documented in this file. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project uses
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

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
