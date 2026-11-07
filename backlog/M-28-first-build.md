---
id: M-28
type: Milestone
title: "First build: sign-in, account, collections and documents as objects"
status: offen
blocked_by: [M-1]
tags: [shell, people, architecture]
created: 2026-11-07
---

The maintainer wants the basics of the mockup to become real: the gate, an account, collections and documents as objects with IDs
(ADR-17, ADR-18). This takes a first part of the shell from M-14, the account from M-5 and the database from M-2. Everything else
that ADR-16 describes is in sight, greyed out. The mockup has a switch, First build, that shows what that looks like. This is a
draft: the talk before the milestone (ADR-14) settles the scope.

**To settle in that talk:**

- The ID model of ADR-18.
- PostgreSQL arriving now instead of with M-2. The order of work in the concept changes with it.
- Where pages live. Either the folder source of M-1 stays as it is and pages are still written in a text editor, or a page made in
  the web is stored in the database. The second needs at least a plain Markdown field to type into, with the full editor left to M-8.
- Who sees what before M-5. As in M-1, everyone past the gate could see everything except the private collection of each user.
- Whether the phone layout belongs in this milestone or follows it. Its mockups come first (T-10).

**Done when this is true, as proposed:**

- The app trusts only the signed user headers of the gate (ADR-9). Without them, nothing but the health endpoint answers.
- The first sign-in creates an account from the gate, with name, mail and groups. Administrators come from a group. The profile shows
  the account and offers signing out through the gate.
- Favorites and settings belong to the account, not to the browser.
- Collections can be made, renamed, described, starred and sorted by dragging, and a collection can sit inside another one and move
  back out (ADR-16). Deleting one that is not empty asks first.
- Pages and folders are objects with an ID, a place, children, a description and tags. They are made with New, moved by dragging,
  and deleting one that is in use asks first.
- Every object opens at its own address, also after a move or a rename, and links from M-1 keep working.
- The shell parts needed for this are built as ADR-16 describes: nav1 to nav3 with their widths and stages, the Workspace view,
  Search with the full-text search of M-1, one window with the general tab and tabs, the document head, the gear with the metadata,
  and the profile with account and settings. Whatever is left for later is greyed out (ADR-17).
- The same clicks hold everywhere, as ADR-17 lists them, and a browser test with real pointer input covers them.
- PostgreSQL holds accounts, collections, objects, places and references. A backup of the database and the folder together
  restores the whole state.
- **Acceptance: the maintainer signs in, builds a few collections with pages, moves and nests them, and opens a copied link in
  another browser.**
