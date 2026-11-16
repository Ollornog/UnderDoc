---
id: M-9
type: Milestone
title: Inline editing
status: offen
tags: [editor, excel]
created: 2026-11-06
---

Embedded data can be edited where it appears.

**Done when this is true:**

- Cells in embedded views can be edited, within permissions.
- Writing back to Excel only happens without a lock, with a matching hash and in named tables.
- Writing back to external sources is optional per source.

**Settled on 2026-11-16 (talk with the maintainer, sprint of that day):** Writing back is switched per source, view and column, with the checks of ADR-27.
