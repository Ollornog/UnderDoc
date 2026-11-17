---
id: ADR-22
type: Decision
title: Reused parts belong to their document, and their end is chosen
status: erledigt
tags: [architecture, spot, editor]
created: 2026-11-16
---

## Context

A page is a list of blocks with IDs (ADR-19), and any block or page can be embedded elsewhere by its address (ADR-18). That
answers how something is reused, but not who it belongs to, what can be reused below a block, and what happens to the places
that use it once the original goes. A catalogue of error messages shows the need: each message with its short explanation is
used in many runbooks, and the catalogue itself may be reorganised or deleted.

## Options

- **Blocks as free objects** with an owner and rights of their own, pages only lists of references: rights per paragraph are
  not manageable, and blocks without a page pile up.
- **A block that is used elsewhere becomes a building block,** an object of its own: one more kind of object, and the user has
  to think about it.
- **A document stays a document.** Its parts belong to it, can be copied or embedded by marking them, and when the original
  goes, its owner chooses what happens to every use.

## Decision

The third option, worked out with the maintainer on 2026-11-16.

- **Every part has exactly one home, its document.** Owner and rights belong to the document, never to a paragraph. Who wrote
  a part comes from its versions (ADR-11). There are no parts without a home.
- **Three units can be reused:**
  - a **block** (paragraph, table, list, code),
  - a **section**: a heading and everything up to the next heading of the same or a higher level. It grows with the original,
    so a paragraph added under the heading shows wherever the section is embedded,
  - a **part inside a paragraph**, down to a few words.
- **Marking decides, nobody types IDs.** The user marks text and chooses *copy* or *embed*. A copy is plain text with no tie;
  its origin is only noted in the version history. Embedding refers to the block, the section or the part.
- **A part inside a paragraph stays where it is.** Its text is not cut out of the paragraph. Two point markers in the source,
  `<!-- part tN -->` and `<!-- /part tN -->`, give it an ID local to its block, addressed as `<object id>#<block id>.<part id>`.
  The markers are anchored in the text, not by position, so typing around them moves nothing. Parts may overlap or nest.
- **A part may cut through formatting.** The embedded part is cut out of the parsed paragraph, and formatting open at its edges
  is closed and opened again, so `**very <!-- part t2 -->important to all** customers<!-- /part t2 -->` embeds as
  `**important to all** customers`. The original stays unchanged. Only atomic elements make a boundary snap to their edge:
  inline code, formulas, the address of a link and raw HTML. A part never leaves its paragraph; anything larger is a block or a
  section.
- **Boundaries show only when needed.** On hover in reading, and while the cursor is inside in the editor, a light outline
  shows the edges of a part, a section or a value, with a label such as "used in 3 places". On a phone, a tap does the same.
  The markers are atomic in the editor and cannot be half deleted. A part ends when all of its text is gone.
- **Values of M-3 are parts too:** the same markers, boundaries and dialog, with a name.
- **Deleting something in use always asks, and the user chooses, with nothing chosen in advance:**
  - **Delete everywhere:** the original goes to the trash. Where it was embedded, a placeholder says who removed it and when,
    instead of the content vanishing quietly. Restoring the original brings every use back.
  - **Delete only the original, every use gets a copy:** each embedding becomes plain text in its page, in its place.
  - **Delete only the original, with a new master:** the user picks one use from the list. The content moves into that page in
    place of its embedding, and every other reference is rewritten to the new place.
  
  Copies and a new master are written only into pages the user may edit. For any other page, its owner gets a notice with the
  same choice (M-16). Deleting everywhere is always allowed, since an embedding is only a view of the original. With a new
  master, the content and the responsibility move to the owner of that page, who gets a notice. Deleting a page with parts in
  use goes through them one by one, with "the same for all".
- **Moving rewrites references.** When a part moves to another page, or gets a new master, its address changes. Every
  reference in `refs` is rewritten in the same transaction, and the old address answers with a redirect, so bookmarks and
  outside links keep working.
- **From GitHub, the ideas, not the model of Git.** History and authorship per block exist already. Proposed changes are
  reviewed and accepted block by block (M-20). Branches across the workspace, merge commits and rebasing are not built for
  pages; real Git stays with the forge module for code.

## Consequences

- The editor shows the boundaries of blocks in use now (M-8). Parts, sections, values and the full delete dialog come with M-3;
  until then deleting a block in use asks and offers to delete or keep it.
- `refs` gains the section and the part as targets, and a table of redirects keeps old addresses alive.
- The Markdown export keeps the part markers, so an import gives every part its ID back.
- Pinning an embedding to a version ("fixed", with a notice when the original changes) is left for later.
- **Built on 2026-11-17** ([docs/parts.md](../docs/parts.md)): the markers, the addresses `<page>#<block>.<part>` and
  `<page>#<block>.section`, a section of blocks (a block that opens with a heading, up to the next block that opens with
  one of the same or a higher level; a heading in the middle of a block does not end it), embedding a part inside a
  paragraph with `:embed{ref=…}`, cutting through formatting with the plain text as the fallback where Markdown cannot
  say it, marking with snapping, copies through the API, the places of use and the list of problems. Then the three
  ways with the placeholder and restoring, copies, a new master with redirects, and notices for M-16. A section that goes
  with a new master takes its blocks along; a heading taken out of a block by editing does not count as deleting its
  section.
