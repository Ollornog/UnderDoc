---
id: M-2
type: Milestone
title: "Files and Excel: the share appears in the web"
status: offen
tags: [read, excel, samba]
created: 2026-11-06
---

Files on a share join the Markdown pages, and every change is traceable. The biggest technical risk of the project sits
here, so T-1 can run before M-1 is finished.

**Done when this is true:**

- Documents, folders and versions live in PostgreSQL and a content store (ADR-13).
- A share with a fixed configuration shows a folder as real files, changes from it become new versions, and the web shows each
  file's share path.
- `underdoc export` writes everything as normal files.
- PDF and other files appear in the tree, and `.xlsx` tables render as native tables.
- Search also covers tables.
- Every change is kept as a version with user and time, and old versions can be restored (ADR-11).
- Access per library. The default is no access.
- Adds the document types Excel file, PowerPoint file with a read-only preview, video that plays in the browser, and image collection, and the share appears in the Files view (ADR-15).
- Files and other objects without front matter can carry tags.
- The facts panel adds who created and changed a document, and its versions.
- **Acceptance: saving in real Excel and in LibreOffice over Samba changes what the web shows.**
