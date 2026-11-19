---
id: M-3
type: Milestone
title: "Single source: values, snippets, embedding"
status: in-arbeit
tags: [spot, mermaid]
created: 2026-11-06
---

One change appears everywhere. Everything is still maintained as files.

**Done when this is true:**

- Values from `_values.yaml` and named Excel cells render inline.
- Snippets from `_snippets/` can be embedded on any number of pages.
- Blocks of a page are embedded by ID since M-28. Here, pasting a block as a copy joins that, with new IDs and no tie to the source.
- Marking text offers copy or embed, for a block, a section or a part inside a paragraph, and a part may cut through
  formatting (ADR-22).
- Boundaries of parts, sections and values show on hover and with the cursor inside (ADR-22).
- Deleting something in use offers delete everywhere, a copy for every use, or a new master picked from the list, with
  nothing chosen in advance; moving rewrites every reference and keeps a redirect (ADR-22).
- Excel tables can be embedded in pages.
- Mermaid diagrams render, also with values and snippets inside.
- Every object shows where it is used, and a list names all broken references.
- Adds the Resources tab with values and snippets (ADR-15).
- The facts panel adds the values, snippets and tables a document uses.

**State 2026-11-17:** parts, sections and values in the Markdown, their addresses, embedding them (also inside a
paragraph), cutting through formatting, marking and copying through the API, where each is used and broken references
are built ([docs/parts.md](../docs/parts.md)). Next: the three ways of deleting, moving with redirects, the boundaries
and marking in the editor and in reading, the Resources tab. **Later the same day:** the three ways of deleting with the
placeholder, restoring, copies, new master, redirects and notices; marking, copying and embedding from the editor, the
boundaries in the editor and while reading, values by name. Open: the Resources tab of the workspace with real values and
snippets (it is still a mock-up of the first build), Excel tables in pages, Mermaid (T-3), the facts panel.

**State 2026-11-19:** the Resources tab with real values and snippets (with how often each is used), the facts panel
with what a page embeds, moving a block to another page from its menu while reading and editing, and the fix of a part
that came back twice when a page left the trash after its part got a new master. Open: Excel tables in pages (with M-2),
Mermaid (T-3), the origin of a text copied through the clipboard in the version of the block it is pasted into, and
"every object shows where it is used" for files and tables once they can be embedded.
