---
type: Task
title: Import a Git-backed Markdown wiki with its history
status: offen
id: T-22
milestone: M-17
created: '2026-11-22'
---
# T-22 — Import a Git-backed Markdown wiki with its history

Teams that started with a wiki of Markdown files in Git move into UnderDoc without losing anything:

- Folders, pages with front matter, attachments and internal links come over as in M-17.
- Tags and front matter fields map to UnderDoc fields; fields that are really records can become a table (ADR-27).
- The Git history becomes the version history of the pages (ADR-11), with author and time.
- Generated sections (backlinks, lists computed when a page is shown) are not imported as text; UnderDoc rebuilds them.
- A dry run reports what would change and which links break.

Embeddings in such a wiki follow UnderDoc's notation without blocks (agreed 2026-11-22 with the first wiki to move):

- Part markers as in docs/parts.md (`<!-- part t2 -->…<!-- /part t2 -->`, also `name="…"`), part IDs unique per page.
- `::embed{ref="<page id>"}` for a whole page, `::embed{ref="<page id>#t2"}` or `:embed{ref="<page id>#t2"}` for a part; no sections, no block markers.
- `id:` in the front matter is the object ID, readable, lowercase, with hyphens.
- The importer gives every page its blocks and rewrites `#t2` to `#bN.t2` for the block the part lives in; a whole page stays as it is.
