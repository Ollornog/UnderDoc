---
id: ADR-13
type: Decision
title: Documents are objects, the folder tree is separate
status: erledigt
tags: [architecture, permissions]
created: 2026-11-06
---

## Context

ADR-3 made the path the identity of a file. But documents have an owner, editors, commenters and viewers, and those must
not depend on where a document happens to sit. Moving a document must not change what it is. Excel users still need
real files on a share, and pages should stay plain Markdown that outlives the application.

## Options

- **Path as identity (ADR-3):** simple, but permissions and links hang on the location, and a move is a new file.
- **Everything as database records:** clean, but pages stop being plain Markdown and the share becomes an afterthought.
- **Documents with identity, content kept unchanged, the share as a projection:** the model is clean and the files stay
  real, but the app has to keep the projection and its store in step.

## Decision

The third option.

- **Document:** a stable ID, a type, one owner (person or group), roles per person or group (editor, commenter, viewer)
  and its versions.
- **Content:** every version is stored unchanged under its hash. Markdown stays exact text, and an Excel file stays the
  exact file. This is also the version store of ADR-11.
- **Folder tree:** separate records. A folder holds default roles for its contents, and a document adds its own. A
  document has one main location, and shortcuts in other folders can come later.
- **Moving** keeps the ID, links, owner and own roles. Inherited roles change, and the UI says so before the move.
- **Folder source:** a folder on disk can feed documents, with the path giving the location and an `id:` in the front
  matter the identity. This is how the read-only phase works without an editor.
- **Share:** chosen folders are projected as real files. A change from outside becomes a new version, and a new file
  becomes a new document whose owner comes from the audit log.
- **Way out:** `underdoc export` writes the whole tree as normal files at any time.

## Consequences

- M-1 separates documents and folders in the code, still in memory. PostgreSQL and the content store arrive with M-2.
- The content store alone is not human-readable, so backups need the database and the store together.
- **Roles on SMB:** owner and editor can write, commenter and viewer can read, and without a role the file should stay
  hidden. Commenting only exists in the web. Whether per-file rights hold on Samba when Office saves through renames is
  unproven (T-1). If they do not, share-enabled folders fall back to rights per folder, and the UI says so.
- A leaving owner must not leave orphans: ownership can be transferred, and a group can be the owner.
- Comments need the database and come with the roles in M-5.
