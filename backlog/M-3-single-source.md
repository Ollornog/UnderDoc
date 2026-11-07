---
id: M-3
type: Milestone
title: "Single source: values, snippets, embedding"
status: offen
tags: [spot, mermaid]
created: 2026-11-06
---

One change appears everywhere. Everything is still maintained as files.

**Done when this is true:**

- Values from `_values.yaml` and named Excel cells render inline.
- Snippets from `_snippets/` can be embedded on any number of pages.
- Blocks of a page are embedded by ID since M-28. Here, pasting a block as a copy joins that, with new IDs and no tie to the source.
- Excel tables can be embedded in pages.
- Mermaid diagrams render, also with values and snippets inside.
- Every object shows where it is used, and a list names all broken references.
- Adds the Resources tab with values and snippets (ADR-15).
- The facts panel adds the values, snippets and tables a document uses.
