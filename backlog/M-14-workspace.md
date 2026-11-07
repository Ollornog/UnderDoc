---
id: M-14
type: Milestone
title: "Workspace: three columns, views, hierarchy, settings and metadata"
status: offen
blocked_by: [M-1]
tags: [ui, shell]
created: 2026-11-06
---

The shell that later modules plug into (ADR-14), with the model of ADR-15 and the choices of ADR-16. It needs no database, so it
runs on the read-only wiki, with pages as the first document type. T-5 settles how it looks before anything is built.

**Done when this is true:**

- A sticky top bar holds the search field and the profile. Nav1, at the far left, holds the views and Search, and at the bottom, set
  apart, Settings and Admin. A module with nothing to show adds no entry.
- Every view uses nav1, nav2 and nav3: view, section, list. What a list opens lands in the same content area.
- The right edge of each column can be dragged, a column pushes the ones to its right, and widths hold in every view. Nav1 and nav2 snap
  to icon, icon with text below, text, and icon with text beside it. Nav3 moves freely.
- In the Workspace view, nav2 holds the favorite collections, and a Collections button shows all of them there with search, stars and
  gears. Until accounts exist, the browser keeps the favorites.
- A click opens only that collection and Ctrl with a click opens several. A setting changes this to a click that marks and a double
  click that opens only that one.
- Open collections stack in nav3, each below a small heading with a separator.
- A page can have subpages. `page.md` next to a folder `page/` appears as one document with children.
- Folders look like documents, apart from their icon.
- Documents open by themselves up to a set number of children, and what the reader opens or closes wins.
- The Files view shows the folder structure on disk, including images, and opening a folder lists its content with size, date
  and place in the workspace.
- The Resources view groups what has no place in a folder. Modules fill it, and an empty group does not appear.
- A gear at the document opens a drawer with contents, width, tags and metadata.
- Contents and width are part of a document's metadata, in the front matter. Contents default to a marker with a drop-down, or
  are a side panel or hidden. Width defaults to slim, or is wide or full, where full uses all the room available and hides
  nothing else. Trying another value in the view changes nothing stored. Saving it from the web comes with editing.
- Content starts at the left, next to the navigation, with a fitting margin, whatever its width.
- Tags from the front matter show at the document and behind the gear, and a click lists every document with that tag.
- One search field in the top bar shows hits in nav3 while typing, with the matching text, and Enter opens the first. Filters by type
  and tags stay behind a button.
- Settings hold the theme with dark mode, the page scale, click behavior, how far lists open by themselves, and the column views. Until
  accounts exist, the browser keeps them.
- The profile shows who is signed in and offers signing out.
- The metadata shows name, type, place in the workspace, path on disk, last change, external links, internal links and backlinks.
- Modules add document types, views and metadata sections through one small interface, proven by a test module.
- A module switched off in the configuration disappears for everyone, and no data is touched.
- **Acceptance: the chosen variant runs on real content, and a browser test with real pointer input covers dragging the columns, at
  desktop and phone width.**
