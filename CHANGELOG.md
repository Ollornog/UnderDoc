# Changelog

All notable changes to this project are documented in this file. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project uses
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

- The interface is the workspace of ADR-16, first part (M-28): a top row with name, search and New, nav1 with the views, nav2
  with the collections, nav3 with the open collection, and the window with its head. What is not built yet stays in sight,
  greyed out. A click in nav3 opens the object in the window without reloading the list, and a double click or the arrow shows
  or hides what is in it. The right edges of the columns can be dragged, nav1 and nav2 snap to four steps, and the browser keeps
  the widths and the theme.
- Every object opens at `/o/<id>` (ADR-18). Links to `/d/<id>` move there.
- The title stands in the head of the window with the breadcrumbs, the tags and the description, not in the content.
- The CSP allows requests to the own origin (`connect-src 'self'`), so the window can be swapped in.
- `scripts/vendor-c22.sh` takes the licence text of the Inter font from C22 instead of keeping its own copy current by hand.

### Added

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
