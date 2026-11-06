---
id: T-9
type: Task
title: Decide which tree decides inherited roles
status: offen
milestone: M-2
blocked_by: [T-1]
tags: [permissions, architecture]
created: 2026-11-06
---

A document inherits roles from where it sits (ADR-13), but it now sits in two trees (ADR-15). Windows only sees the disk, and
if per-file rights do not survive Office saving (T-1), the share can only enforce rights per folder.

- **The workspace decides:** this matches how people think, but the share may have to show less or more than the web.
- **The files tree decides:** the share and the web always agree, but arranging a document never changes who sees it.
- **The workspace decides, and share-enabled folders set a limit** that the UI shows. This is the starting recommendation.

**Done when:** the choice is recorded in a decision before the database schema of M-2 is fixed.
