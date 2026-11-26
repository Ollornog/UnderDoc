---
type: Task
title: 'MCP tools, second round: conflict diff, move a section, search filters, attachment text'
status: offen
sprint: S3
points: 8
id: T-37
milestone: M-31
created: '2026-11-23'
---
# T-37 — MCP tools, second round: conflict diff, move a section, search filters, attachment text

What the team wiki missed most in its use, after the review of T-21 (2026-11-23). Keep the tool set small (they went from 57 to 43 tools and agents got faster).

- **Conflict with diff:** when a write hits a newer version, the answer carries the block diff between the base and the current state, so the agent does not read again.
- **Move a section:** move takes a whole section (#b3 with its sub blocks) to another page, with redirects. PAGE-SIZE advises splitting, today move takes one block only.
- **Search filters:** page kind, author, changed in the last N days; an offset; filter suggestions at the end of the answer (facets exist).
- **Attachment text:** read_file_page also reads attachments of pages; today search finds text in them but no tool reads it.
- **PDF page as image** for scans only (low).

Browser gaps from T-21 belong here too: set the description of a page in the browser; open a PDF at the page of the hit.
