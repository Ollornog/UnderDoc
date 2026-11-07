---
id: M-14
type: Milestone
title: "Workspace: three views, hierarchy, page widths and facts"
status: offen
blocked_by: [M-1]
tags: [ui, shell]
created: 2026-11-06
---

The shell that later modules plug into (ADR-14), with the model of ADR-15 and the choices of ADR-16. It needs no database, so it
runs on the read-only wiki, with pages as the first document type. T-5 settles how it looks before anything is built.

**Done when this is true:**

- A sticky top bar holds the search field. An icon column at the far left holds the views and Search. A module with nothing to
  show adds no entry.
- Each view has its own list next to the icon column, and what a list opens lands in the same content area.
- The Workspace list has a strip of favorite collections, shown as icons, icons with text, or text only. A click opens or closes
  a collection, and open ones stack, each below a small heading with a separator.
- A list of all collections offers a star and a gear per row.
- A page can have subpages. `page.md` next to a folder `page/` appears as one document with children.
- Folders look like documents, apart from their icon.
- Documents open by themselves up to a set number of children, and what the reader opens or closes wins.
- The Files list shows the folder structure on disk, including images, and opening a folder lists its content with size, date
  and place in the workspace.
- The Resources list groups what has no place in a folder. Modules fill it, and an empty group does not appear.
- A gear at the document holds contents, width, tags and metadata.
- Contents and width are part of a document's metadata, in the front matter. Contents default to a marker with a drop-down, or
  are a side panel or hidden. Width defaults to slim, or is wide or full, where full uses all the room available and hides
  nothing else. Trying another value in the view changes nothing stored. Saving it from the web comes with editing.
- Tags from the front matter show at the document and behind the gear, and a click lists every document with that tag.
- Search in the icon column offers today's search, narrowed by type and tags.
- The metadata shows name, type, place in the workspace, path on disk, last change, external links, internal links and backlinks.
- Modules add document types, views and metadata sections through one small interface, proven by a test module.
- **Acceptance: the chosen variant runs on real content, and a browser test covers it at desktop and phone width.**
