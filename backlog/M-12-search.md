---
id: M-12
type: Milestone
title: Proper search
status: in-arbeit
sprint: S2
rest_points: 5
rank: 6
tags: [search]
created: 2026-11-06
---

Finding things is the point of having one place. Replaces the simple search from M-1. The number says nothing about the
order: this can move forward as soon as there is enough content to search.

**Done when this is true:**

- Search covers pages, file contents such as PDF and Office, tables, native lists and cached views.
- Results only show what the reader may read.
- German works: word forms, compounds as far as possible, typing errors.
- Filters by library, type, author and date, with highlighted hits.
- Passwords are never indexed.
- Search offers the strategies exact words and full text (ADR-16).
- A command palette jumps to pages and folders by keyboard.
- A saved search is a document type: a query with strategy, filters and sources, placed in a collection, that runs again each time
  it opens (ADR-16).
- Tags from the front matter and in the text can be searched and listed.
- Text inside images can be found, if measuring shows that OCR is worth its cost.
- PostgreSQL full-text search has been measured against real content before any separate search service is added.

**Settled on 2026-11-16 (talk with the maintainer, sprint of that day):** A search service, Meilisearch by default and OpenSearch as a choice (ADR-24). It comes right after M-8. Error codes are a hard gate, ranking weighs title and headings above the text.

**State 2026-11-19:** built: Meilisearch with a queue in the database, an index per tenant, readers as a filter, error
codes as a hard gate, sections, Ctrl+K, the results page with filters by type, collection, author, date and tags with
highlighted hits and the strategies exact words and full text (#63); OpenSearch as the choice, measured against
Meilisearch on a German corpus (#74); the text of files and of attachments of pages, with where it came from, and text
in images through Tesseract.js in the browser or OCR on the server. Open: tables, native lists and cached views as
sources (they come with M-6 and M-4), the saved search as a document type, the measurement of PostgreSQL full-text
search against real content, readers from groups (M-5), semantic search.
