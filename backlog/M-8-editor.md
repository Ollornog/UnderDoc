---
id: M-8
type: Milestone
title: Web editor
status: offen
tags: [editor]
created: 2026-11-06
---

Pages can be written in the browser as quickly as in a notebook app.

**Done when this is true:**

- The Markdown round trip keeps all directives, proven by a test.
- Autosave, pasting images and fast navigation from notebook to section to page.
- The pipeline fully carries the JavaScript part.
- No free placement on the page.
- A slash menu inserts blocks, and templates start new pages.
- New, from the plus in the top bar or a click on an empty general tab, always lands in a chosen collection or in Quick notes, and
  the last choice stays. Typing and Enter makes a Quick notes card or a Markdown page in the collection. Other types need a
  collection (ADR-16).
- Quick notes land in the private notes, without having to choose a place first. They are a document type: small cards on a board,
  with a field on top to add one.
- Blocks in a page can be aligned left, center or right (ADR-16).
- Contents, width and tags can be saved from the gear at the document.
- Dragging arranges documents and collections in the workspace.
- Writing happens inline, as in Typora: the page looks as it is shown, and the Markdown of a passage appears only where the
  cursor is (the maintainer, 2026-11-08). Until then a block is a plain Markdown field (M-28).
- A bar above the page offers bold, italic, headings, links, lists, code and the like, after Notion and similar editors.
  Every button writes plain Markdown, so the round trip stays whole (ADR-19).
