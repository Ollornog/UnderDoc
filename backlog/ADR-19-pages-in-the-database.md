---
id: ADR-19
type: Decision
title: Pages live in the database, as blocks with IDs
status: erledigt
tags: [architecture, editor, spot]
created: 2026-11-07
---

## Context

M-1 reads pages from Markdown files in a folder, and ADR-13 keeps every page as exact Markdown text. The first build (M-28) makes
pages in the web, and ADR-18 gives a table or paragraph an ID so it can be embedded elsewhere. Several people will edit at once
later (M-10). Plain text files give none of that easily: a part of the text has no identity of its own, and two writers collide
on the whole file.

## Options

- **Markdown files stay the truth:** readable on disk and in Git, but IDs for parts need markers in the text, and editing
  together means merging text.
- **Pages in the database, as blocks:** every block has its ID, embedding refers to it directly, and edits touch one block. The
  files on disk become an export.

## Decision

The second option, as the maintainer chose.

- **A page is a list of blocks in PostgreSQL:** paragraph, heading, list, table, code, and later more. Every block gets its ID when
  it is made (ADR-18), and a table made in a page can be embedded elsewhere by that ID at once.
- **Markdown stays the way in and out.** Import reads Markdown, and export writes it, so `underdoc export` still gives plain files.
  The folder source of M-1 becomes an import.
- **Every version of a page is kept**, block by block, with who changed it and when (ADR-11).
- **Files that are files stay files.** An Excel workbook, a PDF or an image keeps its exact bytes in the content store (ADR-13).

## Consequences

- This replaces two points: pages as exact Markdown text in ADR-13, and the database only from M-2 on in ADR-12. Identity, roles
  and the share of ADR-13 stay as they are.
- Editing together (M-10) works on blocks, so two people editing different blocks never collide.
- On the share, a page appears as a Markdown file. A change made there becomes a new version, and markers in the file keep the
  block IDs. How well that round trip holds is proven in M-2.
- The first build edits a page block by block, each in a plain Markdown field. The full editor stays with M-8.
- A backup needs the database and the content store together.
