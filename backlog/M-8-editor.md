---
id: M-8
type: Milestone
title: Web editor
status: erledigt
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

**Settled on 2026-11-09 (talk with the maintainer):** M-8 comes before M-14 and M-15. It is cut in two: first the core
(writing inline with the bar and the slash menu, autosave, the round trip test, the JavaScript build in the pipeline), then
the rest (Quick notes board, templates, aligning blocks). Pasting images waits for the content store (M-2). The editor is
CodeMirror 6 with a live preview (ADR-21), after a trial of three candidates.

**State 2026-11-09:** the core is built on the branch `editor-core`: one CodeMirror editor per page with the live preview,
the hidden markers with the handles, the bar above the page with Ctrl+B/I/K, the slash menu, pasting addresses, autosave
block by block with the question before deleting what is used elsewhere, a reload instead of overwriting a change made
elsewhere, and the bundle built and compared in the pipeline (docs/editor.md). The round trip test proves that opening
and leaving a page writes nothing. Still open in M-8: Quick notes, templates, aligning blocks, table tools, pasting
images (with M-2), and the rest of the list above.

**Added on 2026-11-16 (ADR-22):** a block used elsewhere shows its boundary and "used in N places" while the cursor is in
it, so whoever writes there knows that other pages change too.

**State 2026-11-17:** part one is merged (#56). Part two adds the rest: Quick notes as a board of cards in the private
notes, New landing in Quick notes or a chosen collection with the last choice kept, templates (UnderDoc's own and pages
offered from their gear), blocks aligned left, center or right, contents and width saved from the gear, the table tools,
and the boundary of a block used elsewhere (ADR-22). Every point above is met except pasting images, which waits for the
content store and goes on with M-2; dragging in the workspace came with M-28. The editor trial (`/dev/editor-trial`) went
on 2026-11-17, as ADR-21 kept it only until M-8 closes.
