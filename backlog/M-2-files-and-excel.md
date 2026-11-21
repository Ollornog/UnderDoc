---
id: M-2
type: Milestone
title: "Files and Excel: the share appears in the web"
status: in-arbeit
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

**Settled on 2026-11-16 (talk with the maintainer, sprint of that day):** Files follow ADR-28 (hybrid store). The content store, uploads, attachments, previews and the text of files for search (ADR-24) come early; Samba, the watcher and T-1 come last in the sprint, with saving from real Excel as the maintainer's acceptance.

**Built on 2026-11-17 (without Samba):** the content store with versions, dedup and a backup that takes it along, the
roots of the view Files (ADR-28 addendum), uploads in chunks that resume, quotas, the thinning of versions, the trash for
files, attachments of pages and "move to the files", and the link from a space to its files ([docs/files.md](../docs/files.md)).
Also built on 2026-11-17: previews of images (on the server) and PDF (pdf.js), Office through an optional Gotenberg
module, the text of files for the search on three ways (browser, server queue, a model of M-13 behind an interface),
each switched by hoster, tenant and user, and images pasted or dropped into the editor as attachments of the page.
**Built on 2026-11-19:** Tesseract.js in the browser with the language data deu and eng, served by UnderDoc itself, and
the text of files and of attachments handed to the search service (M-12). Open in M-2: Excel as native tables (and search
over tables), the facts panel, tags on files, the document types for PowerPoint, video and image collections, Samba and
the watcher (T-1), the acceptance with real Excel.

**Settled on 2026-11-20:** [ADR-30](ADR-30-samba-share.md) says how Samba is built: a working copy on disk only for
shared libraries, the ID as an extended attribute, inotify with a full scan, the save through a temporary file and a
rename, deleting into the trash, the web writing to disk, and no writing from the web while a file is open over SMB.

**Built on 2026-11-20 (ADR-30, [docs/samba.md](../docs/samba.md)):** Samba with the working copy and the watcher: shared
libraries on disk with the ID as an extended attribute, saves as versions (the save through a temporary file and a
rename included), new files and folders, renames, moves and deletions into the trash as the user who made them; the web
writes to disk and is held back while a file is open over SMB, and the facts panel shows each file's share path and who
has it open. Tested against real Samba in the CI. Open in M-2: Excel as native tables (and search over tables), tags on
files, the document types for PowerPoint, video and image collections, and the acceptance with real Excel.

**Settled on 2026-11-20:** [ADR-31](ADR-31-spreadsheets-as-tables.md) says how spreadsheets become tables: a reader of
UnderDoc's own for `.xlsx`/`.xlsm`, `.ods` and `.csv` (excelize weighed and left), cached values without a formula engine,
number formats in the reader's language, hidden stays hidden, limits against bombs and huge sheets, and the cells of the
visible sheets in the search for readers of the file. Read only in this milestone.

**Built on 2026-11-20 (ADR-31, [docs/tables.md](../docs/tables.md)):** the reader of spreadsheets (`.xlsx`, `.xlsm`,
`.ods`, `.csv`) with number formats in the reader's language, held against LibreOffice, and its limits, and the API of
sheets, ranges and CSV. Open: the table view in the workspace, search over tables, embedding (M-3).
Search over tables built the same day: the cells of the visible sheets by sheet and range of rows, for readers of the file.
The table view in the workspace built the same day (sheets as buttons, sticky head, pages of rows, marking, Embed,
CSV). Open in M-2: tags on files, the document types for PowerPoint, video and image collections, and the acceptance
with real Excel (with the tables of ADR-31).
