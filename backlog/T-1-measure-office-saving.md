---
id: T-1
type: Task
title: Measure how Excel and LibreOffice save over Samba
status: offen
milestone: M-2
tags: [excel, samba]
created: 2026-11-06
---

The watcher depends on this, and the only evidence so far is old mailing list threads. Record the file operations with real Excel and LibreOffice on a real Samba share. Also measure whether per-file rights survive a save, and whether files without read access stay hidden (ADR-13).

**Done when:** the observed sequences are documented and turned into test fixtures.
