---
id: ADR-28
type: Decision
title: "Files: the content store keeps the versions, the disk holds the working copy for the share"
status: erledigt
tags: [files, architecture, samba]
created: 2026-11-16
---

## Context

Files that are files stay files (ADR-13, ADR-19). They are reached through the web, Samba and WebDAV, every change is a version
(ADR-11), and tenants are separated (ADR-23). Office programs save over Samba through a temporary file and a rename (T-1).

## Options

- **The disk is the only truth:** simple for Samba, but versions and IDs depend entirely on the watcher.
- **The content store is the only truth,** and Samba sees it through a FUSE file system: one truth, but locks and speed of Office
  over FUSE are a risk.
- **Hybrid:** the content store keeps every version, and folders shared over Samba or WebDAV also exist as real files on disk.

## Decision

Hybrid, chosen by the maintainer on 2026-11-16.

- **Content store:** every version as an immutable blob named by its SHA-256, so equal content is stored once, separated per
  tenant. **Database:** the file object with its ID (ADR-18), place, owner, rights, tags, comments, search text and versions.
  **Disk:** only for libraries shared over Samba or WebDAV, the current state as real files.
- **Libraries:** an own folder per user and team libraries, each shared over Samba or WebDAV or not. Further sources appear as
  more roots (M-27).
- **The ID sticks to the file** as an extended attribute. Where Office saves through a temporary file and a rename, the watcher
  recognises the pattern and carries the ID over.
- A save over Samba becomes a version. Deleting over Samba moves to the trash, and the version stays in the store. A change in the
  web writes to disk. While Office holds a file open, the web says who has it and does not write.
- Rights follow T-9: the workspace decides, and Samba gets rights per folder, per file only where T-1 shows they survive a save.
- Attachments of a page, such as pasted images, belong to the page like its blocks and live only in the store.
- A file can also sit in a collection of the workspace as a reference; its place in the file tree stays the truth for the share.
- Uploads come in chunks and can resume. Previews: images, PDF through pdf.js, Office through LibreOffice headless as an optional
  module.
- Versions are kept in full for a set number of days, then thinned to daily and weekly ones. Each tenant has a quota.
- **Samba never faces the internet:** port 445 only in the LAN or the mesh, and with many tenants WebDAV over HTTPS takes its
  place (M-7). UnderDoc manages Samba users and shares through a small local channel and does not run as root. Each user has
  an app password for SMB.

## Consequences

- Samba comes last in the sprint of 2026-11-16 (the maintainer): LibreOffice is measured in a container, and saving from real
  Excel is the maintainer's acceptance (T-1).
- A backup takes the database and the content store together; the files on disk can be written again from them.
