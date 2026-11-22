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
