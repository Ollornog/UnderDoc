---
id: M-1
type: Milestone
title: "Markdown wiki: a folder becomes a website"
status: offen
tags: [read, markdown]
created: 2026-11-06
---

The smallest useful thing: a folder of Markdown files appears as a readable wiki. No Excel, no Samba, no database, no
editing in the web. Pages are written in any text editor.

**Done when this is true:**

- A folder appears as a tree, and `.md` files render as pages with the C22 design system.
- Pages link to each other, also with `[[Page]]`, and every page shows its backlinks.
- Moving a page keeps links working, through an `id:` in its front matter.
- Images next to a page render.
- A change on disk appears without a restart.
- Simple full-text search covers all pages, from an index in memory.
- It runs as one binary or container with read-only access to the folder, a strict CSP and a health endpoint.
- A sign-in gate sits in front. Everyone past the gate can read everything.
- **Acceptance: a real notes folder is edited in a text editor and read in the browser.**
