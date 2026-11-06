---
id: ADR-11
type: Decision
title: Versions and audit are built in, Seafile is not the base
status: erledigt
tags: [architecture, audit, seafile]
created: 2026-11-06
---

## Context

Files must be reachable from Windows, Linux, Android and iOS, and every change must be traceable: who changed what,
and when. A ready-made file server such as Seafile offers clients for these platforms and file history.

## Options

- **Seafile as the storage base:** native clients and versions, but files are split into blocks in its own store, so
  they are not normal files on disk. That contradicts ADR-3, and the watcher, Samba and Excel on a share would no longer
  work on them. Its audit logs are a feature of the Pro edition.
- **Seafile as an external source:** keeps ADR-3. A library there is read like any other source, and Seafile remains the
  truth for it.
- **Versions and audit in UnderDoc:** every door passes one store and one log, but mobile sync with offline use is not covered.

## Decision

Versions and audit are part of UnderDoc from M-1. Seafile can become an optional external source. UnderDoc does not build
a sync client (ADR-10).

## Consequences

- Every change is stored as a version with hash, path, size, time, user and door. Deleted files can be restored.
- **Who** comes from the door: the app knows it for web, WebDAV and check-out. For SMB only Samba knows it, so the share
  runs `vfs_full_audit` (user, IP, operation, file), and the app matches those entries to the versions it stores.
- If matching fails, the version says “user unknown” instead of guessing.
- Quick successive saves can merge into one version, because the watcher waits for quiet.
- The log is append-only within the app, but whoever has root on the server can alter it. Forwarding it is possible.
- Logging who did what is personal data, so retention is configurable, and employers must check their own rules first.
- Offline use on phones is not covered. If it becomes a requirement, this decision has to be revisited.
