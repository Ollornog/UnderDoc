# Spreadsheets as tables

A spreadsheet in the view Files is read by UnderDoc itself and shows as tables (ADR-31): its visible sheets, the names
and Excel tables on them, and any range, in the reader's language. Nothing is computed and nothing is written: what
shows is what Excel or LibreOffice stored. This page says what is read, how it shows, and the limits that keep a file
from harming the server.

## What is read

| Type | Read | Notes |
|---|---|---|
| `.xlsx`, `.xlsm` | yes | SpreadsheetML; the macros of an `.xlsm` are never opened, links to other workbooks never followed |
| `.ods` | yes | OpenDocument; each cell shows the text its program displayed, so the language of whoever saved it |
| `.csv`, `.tsv` | yes | one sheet named after the file; UTF-8 with or without a byte order mark, else Windows-1252; the separator (comma, semicolon, tab) from the first line; every cell is text |
| `.xls`, `.xlsb` | no | the PDF preview of the Office module, when there is one |

The type comes from the content, as for every file ([files.md](files.md)): a zip named `.xlsx` that holds no workbook
answers why it cannot be read.

## How it shows

- **Values:** the result stored with each formula. A formula stored without its result shows empty, with a hint that
  it was never calculated; errors such as `#DIV/0!` show as they are.
- **Number formats** (SpreadsheetML) are rendered by UnderDoc from each cell's format code, with the separators and the
  names of months and days of the reader's language: German when the browser's first language is German
  (`Accept-Language: de…`), else English. `1234.5` with `#,##0.00 [$€-407]` shows `1.234,50 €` or `1,234.50 €`; the
  built-in short date shows `15.03.2023` or `3/15/2023`. Percent, scientific, fractions, thousands and scaling (`#,##0,"K"`),
  sections with conditions, text sections, elapsed time (`[h]:mm`) and both date systems (1900 with its leap day of 1900,
  1904) are covered. Colours are left out; a negative number whose format would show it only in red keeps its minus.
  Booleans show as `TRUE`/`FALSE` or `WAHR`/`FALSCH`.
- **Merged cells** span their rows and columns; a merge across the edge of a range is cut there.
- **Hidden stays hidden:** hidden and very hidden sheets are not listed and their cells are never read; hidden rows
  (filtered rows too) and hidden columns are left out of every grid and CSV. This is no protection of the data: whoever
  reads the file may download it.
- Fonts, fills, borders, widths, charts, pictures, comments and conditional formats are not taken over.

The number formats are held against LibreOffice: `scripts/sheets-libreoffice.sh` makes a workbook in LibreOffice in a
container (formats, dates, formulas, an error, merges, a hidden row, column and sheet, names, a database range), saves
it as `.xlsx`, `.ods` and `.csv` into `internal/sheet/testdata/libreoffice`, and writes what LibreOffice displays in each
cell; `TestFixturesShowAsLibreOfficeShows` compares every cell. Run it by hand when LibreOffice moves (it uses the image
of T-1).

## In the workspace

A spreadsheet opens in its window as tables (`assets/app/sheets.js`); with the Office module its PDF preview stays one
click away below.

- The visible sheets are a row of buttons above the table; a head of column letters and a column of row numbers stay
  while the table scrolls; rows come in pages of 500 (**Previous rows**, **Next rows**).
- A range is marked by dragging over the cells, by a click and a Shift+click, or with the arrows and Shift and the
  arrows once the table has the focus. **Embed** gives the marked range, or the sheet when nothing is marked, its ID and
  puts its line `::embed{ref=…}` on the clipboard (the line shows below the table as well); **First row is the head**
  goes with it. **Download CSV** downloads the marked range, or the sheet.
- **Names and tables** lists the defined names and Excel tables of the visible sheets, each with **Embed**; a name of
  one cell is a value, which also goes inside a sentence with one colon (`:embed{ref=…}`).
- **Used in pages** lists the tables of the file that have an ID: what became of each in the current version (shows as
  chosen, followed to its new place, its first row changed, no longer to be found, in words, never only as a colour) and
  in how many places it is used, and for a range whose first row changed **Keep it as it is now** (for whoever may edit
  the file). The facts panel names the pages.
- A hit of the search in a spreadsheet opens the file at the sheet and the rows of the hit (`/o/<file>#<sheet>!<range>`).
- Every text from the file goes into the page as text (`textContent`), never as HTML.

## API

| Call | What it answers |
|---|---|
| `GET /api/files/{id}` | `sheets`: the address below, for a type that is read |
| `GET /api/files/{id}/sheets` | the visible sheets with their used range, the defined names on them (`value` for a name of one cell) and the Excel tables, of the current version |
| `GET /api/files/{id}/sheets/grid?sheet=&range=&from=` | a range of a visible sheet (its used range without `range`) as it shows: the visible columns, and up to 500 visible rows from row `from` (counted from 0), or the page that holds row `at`, each cell with its text, its kind and its spans; `more` counts the rows after |
| `GET /api/files/{id}/sheets/csv?sheet=&range=` | the same range as CSV to download (below) |

Each answers only to whoever may read the file (ADR-29), else 404 as for any file. A file that cannot be read answers
422 with the reason in a sentence, never an error of the server. Opening the sheets of a file is logged as opening it.

## CSV without formulas

A CSV of a table puts an apostrophe before every text that starts with `=`, `+`, `-`, `@`, a tab or a carriage return,
unless it is a plain number such as `-89.90`: no spreadsheet program reads it as a formula, and no command of DDE runs
from it. Numbers are written as the plain numbers they hold, booleans as `TRUE`/`FALSE`.

## Limits

Spreadsheets come from anyone who may upload or save over Samba, so each is read as an attack (ADR-31):

| Limit | Value |
|---|---|
| parts of the zip | 2000, counted in the directory before Go's reader reads it into memory |
| one part unpacked | 100 MiB, and at most 200 times its packed size, checked before it is unpacked |
| all parts read, unpacked | 100 MiB |
| sheets | 256 |
| cells with content in a workbook | 1 000 000 (a repeated row or column of OpenDocument counts as what it repeats) |
| rows, columns | 1 048 576, 16 384 |
| shared strings | 1 000 000 |
| a cell's text | 32 767 characters, also as a format shows it (`@@@` shows a text three times) |
| text of the cells of a workbook | 100 MiB, a text counted as often as it stands: a shared string or a repeated cell of a few bytes in the file must not become gigabytes once written out |
| hidden rows and columns kept | 2 097 152 in a workbook; those beyond the last row and column with content are not kept, as LibreOffice writes the rows below the content hidden up to the end of the sheet |
| a format code | 255 characters |
| merged areas | 100 000 |
| time to read one workbook | 30 seconds; at most two at once |
| columns of one page of a grid | 256 (the rest counted as `moreCols`), so one cell far out does not make every view as wide as Excel |
| text of one page of a grid | 8 MiB; the rows beyond are counted as `more`, and the next page starts after the rows the page holds |
| text of a cell in the anchor of a range | 256 characters |
| cells of a range written out as CSV | 1 000 000, empty cells included; a larger range is refused with a sentence |

- A document type in any XML part is refused, and Go's decoder expands no entity besides the five of XML, so no
  external entity is fetched and no entity grows.
- A broken file, or one past a limit, gives its reason; the reason is kept beside the blob (`<sha>-sheet-2.err`), so it
  is not read again. What was read is kept beside the blob too (`<sha>-sheet-2.json.gz`, `-csv-` or `-tsv-` before `sheet` for text, derived, ADR-28) and the last
  eight workbooks in memory. A new version is a new blob and is read anew. Two files with the same bytes share what was
  read, but the one sheet of a CSV is named after the file it is read for, never after the other file.
- A page of a grid costs what the page costs, whatever the sheet holds: merged areas are cut at the edges of the page (a
  merge that runs on shows on the next page as an empty cell of its own), merges over cells another merge covers, as no
  program writes them, are left out, and a cell is looked up once. Writing a range out as CSV, looking for the first row
  of a range and building the text for the search go through the cells once, too.
- A panic of the reader becomes such a reason.
- Every text of a file is escaped in HTML and set as text in the browser.

## Search

The cells of the visible sheets of the current version are given to the search service (ADR-24, Meilisearch or
OpenSearch) as records of the file, with the file's readers (ADR-29): a record for each run of rows of a sheet, up to
1500 words, a row a line, its cells in English. A record is headed by its sheet, and its anchor is `<sheet>!<range>`,
such as `Budget!A1:H12`, so a hit names the sheet and opens the table view at its rows, with the words marked there.

- Hidden sheets, rows and columns give nothing to the search.
- The addresses of cells are not written into the text: a code such as `E1042` would find the cell E1042 of every long
  sheet (ADR-24 looks codes up exactly). A hit names the range of its rows, not the single cell.
- At most 1 MiB of text of a workbook is given, as of any file.
- A spreadsheet that cannot be read as tables is found by the text the server reads, as other files are.
- The source of the search raised its version (5), so every index is filled again once.

## Tables in pages

A sheet, a range, a defined name or an Excel table of a spreadsheet can be embedded in a page (M-3, ADR-31). It gets an
ID local to its file when it is chosen, `x1`, `x2`, …, and is addressed as `<file>#x3`, such as `/o/7k2m9q4x1c#x3`. The
file itself is never changed for it: UnderDoc keeps what the ID points at (`file_tables`, migration 029).

| Kind | Chosen by | Followed through a new version by |
|---|---|---|
| `sheet` | its name | its name; a sheet renamed in Excel by its number (`sheetId`), when both versions were saved by Excel |
| `range` | its sheet and address, such as `A1:D20` | its sheet and address, checked against its first row (the anchor); if that row moved, the range follows it |
| `name` | a defined name on a visible sheet | its name (Excel and LibreOffice move names themselves) |
| `table` | an Excel table, or a database range of OpenDocument | its name; it grows with its rows |

```markdown
::embed{ref="7k2m9q4x1c#x3"}
VAT is :embed{ref="7k2m9q4x1c#x4"} this year.
```

- A line `::embed{ref="<file>#x3"}` shows the table as a block with a link back to the file: at most 200 rows, then a
  link to the rest. An Excel table shows its header row as the head; a sheet, a range or a name when it was chosen with
  `head`.
- `:embed{ref="<file>#x4"}` inside a paragraph shows a table of one cell as a value; anything larger says so in place.
- An embedding shows the current version of the file, rendered for each viewer, in their language. When the file has a
  new version (a save over Samba, an upload, a restore, an import), each table is looked for again the first time it
  shows:
  - **followed:** rows inserted above a range, a name or table that moved, a sheet renamed in Excel: it shows, with
    "Followed from Data!A1:B2 in version 2" above it;
  - **changed:** the first row of a range holds other values and is found nowhere else, or more than once: it shows,
    with a notice that its first row changed, and it is listed among the problems until whoever may edit the file keeps
    it as it is (`POST /api/files/{id}/tables/{x}/keep`) or the page chooses again;
  - **lost:** the sheet, name or table is gone, or its sheet is hidden now: in its place "Table Costs is no longer in
    data.xlsx (version 4 of 20 Nov 2026)", never an empty gap and never the content of an older version. A sheet renamed
    in LibreOffice is lost, since LibreOffice numbers its sheets by their place.
- A file that cannot be read shows its reason in every embedding.

**Rights** (ADR-29): a table is its file's. Whoever reads the file reads, lists and chooses its tables; choosing changes
nothing in the file. A viewer of the page who may not read the file sees "Not to be found: <file>#x3", without a cell
and without the name of the file, the sheet or the table. **Read links:** a page frozen for a link shows an embedded
table only if its file was released for the link (the file is listed among what to release), as it read when the page
was frozen; when the link is opened, a table whose file its maker no longer reads shows "Not shared", as attachments of
a frozen page do.

**Where used:** the file lists the pages that embed its tables (`usedIn` of `GET /api/files/{id}`), each table how many
places use it, and the facts of a page name the tables it embeds (kind `table`). The list of problems names every
embedded table that does not show, and every range whose first row changed.

| Call | What it does |
|---|---|
| `POST /api/files/{id}/tables {"kind", "sheet", "range", "name", "head"}` | gives a table its ID, checked against the current version; the same table again gets the same ID (201 new, 200 known). Answers `id`, `ref`, `url`, `embed` (the line to paste), `label`, `range`, `head`, `state` |
| `GET /api/files/{id}/tables` | the tables of the file that have an ID, each as found in the current version: `state` ok, moved, changed or lost, `moved`, `problem`, `uses` |
| `POST /api/files/{id}/tables/{x}/keep` | a range whose first row changed keeps its place, with its first row as it is now; for whoever may edit the file |

**Export:** the Markdown export writes the directive as it stands; the export of the files keeps the tables of each file
with their IDs in `.underdoc-files.yaml` (`tables:`), and an import gives them back, looked for anew the first time they
show.
