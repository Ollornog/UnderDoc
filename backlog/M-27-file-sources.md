---
id: M-27
type: Milestone
title: Further file sources
status: offen
blocked_by: [M-2, M-5]
tags: [files, sources]
created: 2026-11-07
---

More places for files next to the user's own folder and the share. Only an idea so far: the talk before this milestone (ADR-14)
settles the scope.

**To settle in that talk:**

- Which kinds come first: an SMB share, WebDAV, SFTP, S3.
- Whether a user adds a source for themselves, or only an administrator for a group.
- Where the credentials live (ADR-7), and whether files from a source get versions like those on the share (ADR-11).

**Done when this is true:**

- A source appears as a root in nav2 of the Files view, next to the own folder and the share (ADR-16).
- Every user sees only the sources they may use.
- A source that cannot be reached says so.
