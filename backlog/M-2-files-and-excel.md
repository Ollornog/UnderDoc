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

- A static Samba share serves the library, and the web shows each file's share path.
- PDF and other files appear in the tree, and `.xlsx` tables render as native tables.
- Search also covers tables.
- Every change is kept as a version with user and time, and old versions can be restored (ADR-11).
- Access per library. The default is no access.
- **Acceptance: saving in real Excel and in LibreOffice over Samba changes what the web shows.**
