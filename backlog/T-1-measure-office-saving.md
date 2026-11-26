---
id: T-1
type: Task
title: Measure how Excel and LibreOffice save over Samba
status: in-arbeit
sprint: SA
points: 1
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

**Measured on 2026-11-20, LibreOffice:** LibreOffice 26.8.2 headless saved over a real Samba 4.24 share, mounted with the
kernel's SMB client (SMB 3.1.1) in a container (`scripts/t1-libreoffice.sh`), for `.xlsx`, `.ods`, `.docx` and `.odt`.
smbd's audit recorded every operation; the recordings are the fixtures in
[`internal/share/testdata/t1`](../internal/share/testdata/t1), and `TestRecordedSaves` replays them in every run.

- **The sequence** is the same for all four formats: opening makes the lock file `.~lock.<name>#`; saving writes the whole
  file into `lu<random>.tmp` in the same folder and renames it over the original (`renameat lu….tmp → <name>`), then the
  lock file goes. The saved file is a new inode without the ID; the watcher takes it as a new version of the same object
  and writes the ID on it again. Nothing goes to the trash, and no temporary file becomes an object.
- **Rights per single file do not survive a save:** an ACL set on the file alone (another user taken off) was gone
  afterwards, since the new file inherits the default ACL of its folder. So Samba keeps rights per folder (ADR-29,
  ADR-30), and the rights dialog says so.
- **A file without read access stays hidden** (`hide unreadable`): the other user did not see it in the listing, and saw
  it again after the save, when it had the rights of its folder.
- Open: the same with **real Excel** on Windows (the maintainer's acceptance). Excel is known from public sources to save
  through a temporary file and renames too (`TestOfficeSaveBecomesAVersion` takes that pattern, not measured here). The
  measurement uses LibreOffice on Linux; LibreOffice on Windows may save differently.

**To check with real Excel as well (B-2, 2026-11-24):** hide all rows below the data (select the rows, Hide), save over
Samba: the web shows the same rows as Excel. If Excel writes `<sheetFormatPr zeroHeight="1"/>`, note whether a row with
content can then be hidden without `hidden="1"` of its own; UnderDoc reads it after ECMA-376 (only rows written out are
shown). Download a CSV of a sheet with a text `＝1+1` and one with ` =1+1` and open it in Excel: both show as text.
