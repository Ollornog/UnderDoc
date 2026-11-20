---
id: T-1
type: Task
title: Measure how Excel and LibreOffice save over Samba
status: in-arbeit
milestone: M-2
tags: [excel, samba]
created: 2026-11-06
---

The watcher depends on this, and the only evidence so far is old mailing list threads. Record the file operations with real Excel and LibreOffice on a real Samba share. Also measure whether per-file rights survive a save, and whether files without read access stay hidden (ADR-13).

**Done when:** the observed sequences are documented and turned into test fixtures.

**How it is built (2026-11-20):** [ADR-30](ADR-30-samba-share.md) settles the working copy, the watcher and the
companion. LibreOffice is measured in a container against the real companion, with smbd's audit (`vfs_full_audit`) and
the operations on disk recorded; the recordings become the fixtures of the watcher's tests. Real Excel stays the
maintainer's acceptance.
