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
