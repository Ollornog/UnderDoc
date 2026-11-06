---
id: B-1
type: Bug
title: Links to folders count as broken
status: offen
milestone: M-1
tags: [links]
created: 2026-11-06
---

**Repro:** in a page, write `[backlog](backlog/)` or `[backlog](../backlog/)`, where that folder exists, and open `/problems`.

**Expected:** the link works and is not listed, for example by opening the folder in the tree.

**Actual:** the page shows it as broken, and `/problems` lists “no such page or file”. The repository's own `README.md` and
`docs/concept.md` both do this.
