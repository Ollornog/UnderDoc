---
id: T-4
type: Task
title: Store versions and record who changed what
status: erledigt
points: 1
milestone: M-2
blocked_by: [T-1]
tags: [audit, samba]
created: 2026-11-06
---

Build the version store and the change log as described in ADR-11, including the SMB case through `vfs_full_audit`.

**Done when:** after saving through Samba from Excel and LibreOffice, the web shows the version with the right user and
time, an older version can be restored, and a test covers an entry that cannot be matched.

**Closed on 2026-11-24:** built with M-2: the content store keeps every version with user and time and restores old ones (2026-11-17), and the Samba companion reads smbd's audit (`vfs_full_audit`, `cmd/underdoc-samba/audit.go`, docs/samba.md) to name who changed a file over the share.
