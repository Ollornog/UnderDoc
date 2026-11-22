---
type: Task
title: Extract the spreadsheet reader as its own public Go library (gridport)
status: offen
id: T-13
milestone: M-2
created: '2026-11-22'
---
# T-13 — Extract the spreadsheet reader as its own public Go library (gridport)

The reader for xlsx, xlsm, ods, csv and tsv (ADR-31) becomes its own public Go repository, gridport, under the MIT license, and UnderDoc takes it as a dependency.

- Reading now; writing (creating new xlsx, ods and csv files) later in the same repository, together with the data-exchange decision (T-14).
- Editing existing files is not a goal: that stays with Excel or LibreOffice over the share (ADR-30).
- The package depends only on internal/files. The cache, HTML(), anchors and SearchText stay in UnderDoc.
- Extract only after writing is designed, so the public API does not change twice.

Done when gridport is public with its own tests and UnderDoc reads spreadsheets through it.
