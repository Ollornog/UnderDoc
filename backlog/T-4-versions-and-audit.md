---
id: T-4
type: Task
title: Store versions and record who changed what
status: offen
milestone: M-1
blocked_by: [T-1]
tags: [audit, samba]
created: 2026-11-06
---

Build the version store and the change log as described in ADR-11, including the SMB case through `vfs_full_audit`.

**Done when:** after saving through Samba from Excel and LibreOffice, the web shows the version with the right user and
time, an older version can be restored, and a test covers an entry that cannot be matched.
