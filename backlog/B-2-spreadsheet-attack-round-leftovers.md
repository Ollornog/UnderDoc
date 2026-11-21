---
id: B-2
type: Bug
title: Spreadsheets, what the attack round of ADR-31 left open
status: offen
milestone: M-2
tags: [files, excel, security]
created: 2026-11-21
---

The attack round on spreadsheets as tables (ADR-31, 2026-11-21) fixed what a file could do to memory, CPU, the database
and a CSV download. Found and not fixed there:

1. **The cache of read workbooks is bounded by count, not by size.** It keeps the last eight workbooks; each may hold up
   to 100 MiB of text and a million cells, so eight files at the limits keep about a gigabyte in memory.
   **Expected:** the cache counts what it holds and drops the oldest past a size. **Actual:** eight, whatever their size.
2. **Rows hidden by default are not read as hidden.** SpreadsheetML can mark every row of a sheet hidden by default
   (`<sheetFormatPr zeroHeight="1"/>`, ECMA-376 18.3.1.81); the reader looks only at `hidden` on each row. To check with a
   file saved by Excel after hiding all rows below the data: whether a row with content can be hidden that way without
   `hidden="1"` of its own. Not checked in the round (no Excel at hand).
3. **The text of a merged area whose first cell is hidden** shows in the first visible cell of the merge (as Excel
   shows a merged cell that is partly hidden). The search and a CSV leave it out, since there the cell is hidden. Not a
   leak (hidden is no protection, ADR-31), but the three disagree.
4. **CSV and look-alike characters:** a text starting with a full-width `＝` or with a space before `=` is written as it
   is. LibreOffice 26.8 reads both as text (checked by hand); Excel was not checked.
