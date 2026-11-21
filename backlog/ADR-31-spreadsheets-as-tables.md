---
id: ADR-31
type: Decision
title: "Spreadsheets as native tables: read by UnderDoc, embedded by a local ID, never quietly wrong"
status: erledigt
tags: [files, excel, spot, search, security, architecture]
created: 2026-11-20
---

## Context

M-2 asks that `.xlsx` files render as native tables and that search covers them; M-3 asks that Excel tables can be
embedded in pages, that named Excel cells render inline as values, and that every object shows where it is used. Files
live in the content store with versions (ADR-28), a new version arrives by upload, by a save over Samba (ADR-30), by
restoring an older one or by an import. Pages embed blocks, sections and parts by their address (ADR-18, ADR-22,
[docs/parts.md](../docs/parts.md)), live and never wider than the viewer may see. Roles hold on files as on documents
(ADR-29). The search keeps one record per section with its readers as tokens (ADR-24).

Today a spreadsheet is a file with a PDF preview through the optional Office module and with its text, read on the
server as one run of words, in the search. This decision says how a spreadsheet becomes tables, before the code.

## Options

**Who reads the file:**

- **excelize** (`github.com/xuri/excelize/v2`, v2.11.1 when this was written, BSD-3-Clause, maintained): the common Go
  library for OOXML. It reads the whole file into memory and unpacks every part on open; it formats numbers in the
  cultures en-US, ja-JP, ko-KR, zh-CN and zh-TW only, so a German reader would see `1,234.56` where Excel shows
  `1.234,56`; its streaming reader gives values without their style, so formatting in another language needs the whole
  sheet in memory; it does not read OpenDocument; it writes temporary files for large parts.
- **LibreOffice through the Office module** (Gotenberg), converting to CSV or HTML: right formatting, but the module is
  optional, one sheet per call, no addresses of cells, no names or tables, and a service far larger than UnderDoc in the
  path of every view.
- **A reader of UnderDoc's own** for the two open standards (ECMA-376 SpreadsheetML, OpenDocument) and CSV: streaming
  XML with the limits checked inside the loop, number formats rendered in the reader's language, and the zip handling
  that already reads the text of Office files with limits against bombs (`internal/files`). More code to keep, all of it
  pure Go without a new dependency.

**How a table is addressed:**

- **By what the file says,** `<file>#Sheet1!A1:D20` in the page: readable, but a renamed sheet or a moved range breaks every
  embedding at once, or worse, shows other cells without a word.
- **A marker in the file:** UnderDoc never writes into a spreadsheet only to keep its own bookkeeping.
- **A local ID kept by UnderDoc** for each table that something uses, with what it points at beside it: the address stays,
  and when the file changes there is one place to follow it or to say that it is lost.

## Decision

A reader of UnderDoc's own, tables addressed by local IDs that UnderDoc keeps beside the file, rendered live for each
viewer from the current version, read only in this milestone. In detail:

### Formats

- **`.xlsx` and `.xlsm`** (SpreadsheetML): read. The macros of an `.xlsm` (`vbaProject.bin`) are never opened, never
  run; links to other workbooks are never followed, their cached values show.
- **`.ods`** (OpenDocument): read. An ODS file stores, beside each value, the text the saving program displayed; UnderDoc
  shows that text as it is, so an ODS file shows the formatting and language of whoever saved it.
- **`.csv`** (and `.tsv`): read as one sheet named after the file, UTF-8 with or without a byte order mark, otherwise
  Windows-1252; the separator is found from the first lines (comma, semicolon, tab). No formats.
- **`.xls`, `.xlsb`** and other binary formats: not read; they keep the PDF preview of the Office module.
- The type comes from the content as for every file (docs/files.md): a zip with `xl/workbook.xml` is SpreadsheetML,
  with the mime type `application/vnd.oasis.opendocument.spreadsheet` OpenDocument; CSV only by its name and plain text.

### What a table is

Four kinds of table, all inside one spreadsheet file:

| Kind | What it is | Follows changes of the file by |
|---|---|---|
| **sheet** | a visible sheet, its used range | its name |
| **range** | a rectangle of cells of a sheet, such as `A1:D20` | its sheet, its address and its first row |
| **name** | a defined name of the workbook or of a sheet that points at one rectangle (Excel "Name Manager", LibreOffice "named range") | its name |
| **table** | an Excel table (`ListObject`, "Format as Table") with its header row, or an OpenDocument database range | its name |

A name that points at a single cell is a **value** (M-3): it shows inline in a paragraph. A name that points at several
rectangles, at another workbook or at a formula without cells is not offered.

### IDs and addresses (ADR-18)

- The file is the object; a table is a part of it with an ID local to the file, `x1`, `x2`, …, addressed as
  `<file>#x3`, such as `/o/7k2m9q4x1c#x3`. Like a part of a page it gets its ID when it is chosen, by embedding it or by
  copying its address; choosing the same table again gives the same ID. Nobody types IDs.
- UnderDoc keeps, for each ID, what it points at (the kind, the sheet, the address or the name), which version of the
  file it last found it in, and for a range the values of its first row when it was chosen (the anchor). The file itself
  is never changed for it.
- Owner and rights are the file's, as a part of a page has its page's (ADR-22). The table view lists the sheets, names
  and tables of a file by their names; IDs exist only for what something uses.

### Embedding

- A line `::embed{ref="<file>#x3"}` on its own embeds a table as a block, with a link back to the file and the table's
  name; `:embed{ref="<file>#x4"}` inside a paragraph embeds a value, as a value of a page shows (docs/parts.md). A table
  of more than one cell inside a paragraph says so in place.
- An embedded table shows at most 200 rows and says how many more there are, with a link to the table view. An Excel
  table shows its header row as the head of the table; a sheet, a range and a name show their first row as a head when
  that was chosen with them.
- Every embedding is kept in `refs` with its target `x3`, so the file and each of its tables show where they are used,
  and the list of problems names every embedding that does not resolve.
- The workspace offers embedding from the menu of the file (a sheet, a name, a table) and from a range marked in the
  table view; the address goes to the clipboard, and pasting it alone on a line embeds it, as for parts.

### When the file changes

A save over Samba, an upload, restoring a version or an import makes a new current version. An embedding always shows
the current version, rendered again for each viewer; nothing is copied into the page. When the current version is
newer than the one a table was last found in, UnderDoc looks for it again:

- **A sheet, a name or a table** is found by its name. Excel and LibreOffice move names and tables themselves when rows or
  columns are inserted, and an Excel table grows with its rows, so this is the way that keeps working. A sheet renamed
  in Excel keeps its internal number (`sheetId`), and Excel never gives a number twice; so a sheet that is gone by name
  is followed to the sheet that carries its old number, when no sheet has the old name and both versions were saved by
  Excel (`docProps/app.xml`). LibreOffice numbers the sheets by their place on every save, so a number there says
  nothing, and a sheet renamed in LibreOffice is lost (and told so).
- **A range** is found at its address and checked against its anchor. When the first row still holds the values it held,
  it shows. When not, the sheet is searched for that row, as wide as the range: found exactly once, the range follows
  it (its new address is kept, and the facts say where it moved from); found nowhere or more than once, the range shows
  with a notice above it that its first row changed since it was chosen, and whoever may change the page can keep it as
  it is now (a new anchor) or choose again. It is never shown as if nothing happened.
- **What is gone** shows in place what is missing and since which version: "Table Budget is no longer in plan.xlsx
  (version 7 of 20 Nov 2026)", never an empty gap and never the content of an older version.
- **A file that cannot be read** (broken, too large, a type that is no spreadsheet any more) shows the reason in place,
  in every embedding and in its table view.
- A followed table keeps its ID, so every embedding follows with it. Who changed the file and when comes from its
  versions (ADR-11).

### Values and formats

- **Formulas are not computed.** A cell shows the result that Excel or LibreOffice stored with it. A formula stored
  without a result, as some programs write files, shows empty with the hint that it was never calculated. Errors such as
  `#DIV/0!` show as they are.
- **Number formats** of SpreadsheetML are rendered by UnderDoc from the format code of each cell (ECMA-376, part 1,
  18.8.30–31): the built-in formats by their meaning, and custom codes with sections and conditions, digits (`0`, `#`,
  `?`), thousands and scaling, percent, scientific, fractions, text (`@`), literals and currencies (`[$€-407]`), dates and
  times with elapsed hours (`[h]`) and both date systems (1900 with its leap day of 1900, 1904). Separators and the names
  of months and days follow the reader's language (the browser's first language: German, otherwise English); the
  built-in date formats are the reader's short date. Colours of a format are left out, and a negative number that a
  format would mark only by colour keeps its minus sign, so nothing depends on colour.
- **Merged cells** span their rows and columns; a merge that crosses the edge of a range is cut at the edge.
- **Hidden stays hidden:** hidden and very hidden sheets, hidden rows (filtered rows too) and hidden columns are not
  shown, not offered for embedding, not given to the search and not written into a CSV. A name or table on a hidden sheet
  is not offered either. This is not a protection of the data: whoever may read the file may download it.
- Fonts, fills, borders, widths, charts, pictures, comments and conditional formats are not taken over; tables take the
  look of the workspace (C22). Numbers align to the right.

### Read only

This milestone only reads. Writing cells from the web comes later, and when it comes, it writes a new version of the
file, held back while the file is open over SMB, as the web writes to disk (ADR-30).

### Rights (ADR-29)

- A table belongs to its file: whoever reads the file reads its tables, and nobody else. Embedding bypasses nothing: a
  viewer who may not read the file sees a placeholder, "Not to be found", without any cell, without the name of the file,
  of the sheet or of the table.
- Choosing a table (giving it an ID) is reading; it changes nothing in the file and is open to every reader of the file,
  as embedding is.
- **Where used:** the file and each of its tables list the pages that embed them, among the pages the viewer sees, and
  how many more there are.
- **Read links:** a page frozen for a link shows an embedded table only if its file was released for the link, as
  embedded pages are (`/shareables` lists the file), and as it read when the page was frozen. When the link is opened,
  a table whose file its maker no longer reads shows "Not shared", the rule of attachments in links (2026-11-20).
- **Export:** the Markdown export writes the directive as it stands; the export of the files keeps the tables of each
  file with their IDs in `.underdoc-files.yaml`, so an import gives them back.
- **Search:** cells are found only by readers of the file (below).

### Search (ADR-24, M-12)

The cells of the visible sheets of the current version go to the search service as records of the file, one or more
per sheet, each cell with its address (`Budget!B7`) and its formatted text, readers as for the file. A hit names the
sheet and the cell and opens the table view there. The tokens of codes (ADR-24) work on cells as on text. The text the
server reads for search stays for every other kind of file; a spreadsheet the reader reads is indexed by its cells
instead. Meilisearch and OpenSearch take the same records.

### Defence

Spreadsheets come from anyone who may upload or save over Samba, so the reader treats each file as an attack:

- **Zip:** at most 2000 parts; a part is checked against its packed size before a byte is unpacked, at most 100 MiB and
  200 times its packed size; at most 100 MiB unpacked in all; the sizes of the zip directory can lie, so reading never
  goes beyond what they say (the limits of `internal/files`).
- **XML:** Go's decoder reads no document type definition and expands no entity besides the five of XML, so no external
  entity is fetched and no entity grows; a part that declares a document type is refused all the same.
- **Size:** at most 256 sheets, 1 000 000 cells with content in a workbook, 1 048 576 rows and 16 384 columns (Excel's
  limits; a repeated row or column of OpenDocument counts as what it repeats), 1 000 000 shared strings, 32 767
  characters in a cell, 255 in a format code, 100 000 merged areas. More is refused with the limit it passed.
- **Time:** reading a workbook stops after 30 seconds; at most two are read at once. What was read is kept beside the
  blob (derived, ADR-28) and in a small cache in memory, so a page with embeddings does not read the file again.
- **Failure:** a file that breaks a limit or the reader gives a reason to see, never an error of the server; a panic of
  the reader is caught and becomes such a reason.
- **Output:** every text from a file, cells, names of sheets and tables and the literals of format codes, is escaped in
  HTML on the server and set as text in the browser, never as HTML. A CSV written from a table (download from the table
  view) puts an apostrophe before every text cell that starts with `=`, `+`, `-`, `@`, a tab or a carriage return, so no
  spreadsheet program reads it as a formula; numbers are written as plain numbers, which never start a formula.

### Web

- A spreadsheet opens in the table view: its visible sheets as tabs, a head with the column letters and the row numbers
  that stays while scrolling, and rows in pages of 500. The PDF preview stays one click away when the Office module is
  there.
- A range is marked with the mouse or the keyboard; the menu of the file offers its sheets, names and tables for
  embedding, and downloading the shown sheet or range as CSV.
- The facts panel of the file adds its tables and where each is used.
- The look comes from C22 classes; what C22 lacks is asked of C22, not built beside it.

## Consequences

- A new package reads spreadsheets into a model of sheets, rows and cells with their formats, and formats them for a
  language; a table `file_tables` keeps the IDs with what they point at, under row-level security like every table
  (ADR-23).
- The address grammar of embeddings gains `#x<n>` for tables of a file, beside `#b<n>` for blocks of a page.
- Fixtures are real files saved by LibreOffice in a container, as for T-1, and hand-made files for the limits (bombs,
  entities, huge sheets). **Acceptance: the maintainer saves a workbook in real Excel over Samba, and the table in the
  page follows.**
- The server's text of a spreadsheet gives way to its cells in the search; the search source raises its version, so
  every index is filled again once.
- Open for later: writing cells from the web; pinning an embedding to a version; reading `.xls`; formats of OpenDocument
  in the reader's language; charts.

## Built on 2026-11-20, and settled while building

Built in four steps ([docs/tables.md](../docs/tables.md)): the reader and its API, tables in pages, the search, the table
view in the workspace. Settled on the way:

- **Keeping a changed range** is for whoever may **edit the file**, not whoever may change a page: the ID is the
  file's, and every page that embeds it follows. Whoever may change a page can always choose again.
- **A hit of the search names the sheet and the range of its rows,** not the single cell: the addresses of cells stay
  out of the indexed text, since ADR-24 looks codes up exactly and `E1042` would find the cell E1042 of every long sheet.
  The table view opens at those rows.
- **Numbers align to the right** in embedded tables and in read links; inside the workspace C22 sets the alignment of
  table cells itself, so they align to the right there once C22 lets a cell say so (asked of C22).
- **The sheets** show as a segmented control of the workspace rather than tabs; C22's tabs are not styled inside it.
- **A lone cell far out** (XFD1048576) makes the used range Excel's whole grid: a page of a grid is at most 256 columns
  wide (the rest counted), the anchor of a range at most 256 texts, a CSV at most a million cells, and the text for the
  search goes through the cells there are, never the empty cells of a rectangle. Found while building.
- The table of the IDs is `file_tables` (migration 029); one cache of read workbooks serves the web and the search
  (`sheet.Cache`).
