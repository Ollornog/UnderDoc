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

## API

| Call | What it answers |
|---|---|
| `GET /api/files/{id}` | `sheets`: the address below, for a type that is read |
| `GET /api/files/{id}/sheets` | the visible sheets with their used range, the defined names on them (`value` for a name of one cell) and the Excel tables, of the current version |
| `GET /api/files/{id}/sheets/grid?sheet=&range=&from=` | a range of a visible sheet (its used range without `range`) as it shows: the visible columns, and up to 500 visible rows from row `from` (counted from 0), each cell with its text, its kind and its spans; `more` counts the rows after |
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
| parts of the zip | 2000 |
| one part unpacked | 100 MiB, and at most 200 times its packed size, checked before it is unpacked |
| all parts read, unpacked | 100 MiB |
| sheets | 256 |
| cells with content in a workbook | 1 000 000 (a repeated row or column of OpenDocument counts as what it repeats) |
| rows, columns | 1 048 576, 16 384 |
| shared strings | 1 000 000 |
| a cell's text | 32 767 characters |
| a format code | 255 characters |
| merged areas | 100 000 |
| time to read one workbook | 30 seconds; at most two at once |
| columns of one page of a grid | 256 (the rest counted as `moreCols`), so one cell far out does not make every view as wide as Excel |
| cells of a range written out as CSV | 1 000 000, empty cells included; a larger range is refused with a sentence |

- A document type in any XML part is refused, and Go's decoder expands no entity besides the five of XML, so no
  external entity is fetched and no entity grows.
- A broken file, or one past a limit, gives its reason; the reason is kept beside the blob (`<sha>-sheet-1.err`), so it
  is not read again. What was read is kept beside the blob too (`<sha>-sheet-1.json.gz`, derived, ADR-28) and the last
  eight workbooks in memory. A new version is a new blob and is read anew.
- A panic of the reader becomes such a reason.
- Every text of a file is escaped in HTML and set as text in the browser.
