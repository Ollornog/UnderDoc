# Files

Files that are files stay files (ADR-13, ADR-19, ADR-28). The view **Files** shows them next to the documents, with
versions, a trash, quotas and uploads that resume. Samba shows them as network drives ([samba.md](samba.md), ADR-30);
WebDAV comes later (M-7).

## Setup

Files need the database and a directory for the content store:

```bash
export UNDERDOC_DATABASE_URL=postgres://underdoc@db/underdoc
export UNDERDOC_STORE_DIR=/var/lib/underdoc/store
```

| Setting | Meaning | Default |
|---|---|---|
| `UNDERDOC_STORE_DIR` | Directory of the content store. Without it, there are no files | none |
| `UNDERDOC_FILES_MAX_SIZE` | Largest size of one file | `2G` |
| `UNDERDOC_FILES_QUOTA` | Quota of the whole tenant: all files, versions and attachments | `0`, none |
| `UNDERDOC_FILES_USER_QUOTA` | Quota of a user's folder that has none of its own | `10G` |
| `UNDERDOC_FILES_COLLECTION_QUOTA` | Quota of a collection that has none of its own | `0`, none |
| `UNDERDOC_FILES_KEEP_ALL_DAYS` | Every version is kept this many days | `30` |
| `UNDERDOC_FILES_KEEP_DAILY_DAYS` | After that, the last version of each day until this age | `90` |
| `UNDERDOC_FILES_KEEP_WEEKLY_DAYS` | After that, the last version of each week until this age; `0` keeps them for good | `0` |

Sizes take `K`, `M`, `G` and `T` for powers of 1024. The current version of a file is always kept, however old it is.
Days count in UTC.

## The content store

Every version of a file and every attachment of a page is a blob named by the SHA-256 of its content, so equal content
is stored once. Each tenant has a directory of its own (ADR-23):

```text
<store>/<tenant>/blobs/ab/cd/<sha256>   read only once written
<store>/<tenant>/uploads/<id>           uploads in progress
<store>/<tenant>/derived/<sha256>…      previews and other things made from a blob
```

The tables of the files carry their tenant and are kept apart by row-level security like all others (ADR-23,
[tenants.md](tenants.md)); the upkeep runs for each tenant. No name a user gives ever becomes a path in the store. The database knows which blob belongs to what: the file object
with its ID (ADR-18), place, owner, tags, comments and versions. Once an hour UnderDoc ends uploads that got nothing for a
day, thins the versions, and takes blobs nothing refers to any more out of the store; once a day it also removes blobs
older than a day that the database does not know, such as one whose upload failed at its last step.

## Roots

Files has three kinds of roots, all of them collections in an area of their own (ADR-28, addendum of 2026-11-17):

- **The folder of the tenant** ("Company"): made when an administrator first opens Files, and owned by them. The whole
  tenant reads it; the group named as its writers edits it, at first the group of administrators
  (`UNDERDOC_ADMIN_GROUP`).
- **My files:** a folder for each account, private to it.
- **Collections:** made by anyone, and theirs alone until they share them in the step "Share with" or in their rights
  (ADR-29, 2026-11-19; before, the whole tenant edited a new one).

Roles for persons, groups and the tenant hold here as among the documents, on every root, folder and file
([rights.md](rights.md), ADR-29); the writers of the folder of the tenant are a group with the role edit there. The
folder of the tenant and the folders of users are never deleted or moved. Files and folders stay in the view Files, and pages stay among the documents: a move or copy between the two is
refused.

## Uploads

An upload comes in chunks of up to 8 MiB and can resume after a break:

```text
POST   /api/uploads               {"parent": "<folder>", "name": "plan.pdf", "size": 1234567}   → {"id", "received", "chunk"}
PATCH  /api/uploads/<id>          header Upload-Offset: <received>, body: the next chunk          → {"received"}
GET    /api/uploads/<id>          how much has come, to resume                                    → {"received", "size"}
POST   /api/uploads/<id>/finish   the file, or its new version                                    → {"id", "version", "type"}
DELETE /api/uploads/<id>          cancel
```

Instead of `parent`, `file` uploads a new version of a file, and `page` an attachment of a page. A chunk at another
offset than what has come is refused with `409` and the offset to go on from. The size is declared at the start: it is
checked against the largest size and the quotas then, and counts against them until the upload ends, so many uploads at
once cannot pass a quota. The workspace keeps the ID of each upload in the browser, so choosing the same file again
resumes it.

A file uploaded under the name of a file in the same folder, without regard to case, becomes a new version of that file.
An older version comes back as a new version, so nothing is lost. The versions are for whoever may edit the file; whoever
only reads it gets the current one ([rights.md](rights.md)). An upload through a link to upload only never becomes a
version: it takes a name nothing in the folder has when the file is made, with the folder locked meanwhile.

## What a file is, and how it is served

The type comes from the content, never from what the browser says: a PNG named `report.pdf` is a PNG. The name only
tells apart what the content cannot, such as the kinds of Office files, which are all zip archives, or Markdown from
plain text. Raster images, plain text, sound and video are shown in the browser; everything else is downloaded, and above
all HTML, SVG and XML, which can carry scripts. Everything served from the store has a policy under which nothing runs
(`sandbox`), and its type is never sniffed. Content is only served through a file or attachment the viewer may see.

## Previews

- **Images** (PNG, JPEG, GIF, WebP, BMP): a small picture is made on the server with Go alone, the first time it is
  asked for (`/api/files/<id>/thumb?size=256` or `1024`), and kept beside the blob. An image says its size in its
  header; one of more than 50 million pixels, such as a decompression bomb, gets no preview and is never decoded. At most
  two previews are made at once.
- **PDF** shows in the window through pdf.js, built from its lockfile in `web/pdfjs` and served by UnderDoc itself with
  its fonts, character maps and decoders. Only the pages are drawn, the first 30; no layer of links or forms, no scripts
  of the PDF, and no `eval`.
- **Office files** (Word, Excel, PowerPoint, OpenDocument) show the same way once an Office module converts them to PDF.
  LibreOffice is far larger than UnderDoc, so it is not in the image: a Gotenberg service (MIT) next
  to UnderDoc does it (`UNDERDOC_OFFICE_URL`, for example `http://127.0.0.1:3000`), and the PDF is kept beside the blob.
  Without the module, Office files are downloaded.

| Setting | Meaning | Default |
|---|---|---|
| `UNDERDOC_OFFICE_URL` | The Office module for previews, a Gotenberg service | none: no Office previews |

- **Spreadsheets** (`.xlsx`, `.xlsm`, `.ods`, `.csv`) are read by UnderDoc itself as tables, without the Office module:
  [tables.md](tables.md).

## Text for the search

The text of files reaches the search three ways (ADR-24), each switched on in layers: the hoster allows it
(`UNDERDOC_TEXT_WAYS`), the tenant leaves it on or switches it off, and each user does the same for their own uploads
(`GET`/`PUT /api/files/text-settings`; the tenant's part only administrators). Each text is kept with where it came from.

- **Browser:** after an upload, the uploader's browser reads the text of a PDF with pdf.js, and the text of an image
  (PNG, JPEG, WebP, BMP) with Tesseract.js in German and English, and sends it (`PUT /api/files/<id>/text`).
  Tesseract.js, its core (WebAssembly with SIMD, the LSTM engine only) and the language data `deu` and `eng`
  (tessdata_best, int) are built from the lockfile in `web/tesseract` and served by UnderDoc itself, about 7 MB, loaded
  only when an image is uploaded; Tesseract.js would fetch them from a CDN by default, the reader names every path
  instead. Its worker is the one script whose policy allows WebAssembly (`'wasm-unsafe-eval'`); the pages keep theirs.
  A browser without WebAssembly SIMD leaves the text to the server. Scanned PDF are not read in the browser.
- **Server:** a queue reads the text of PDF, Word, Excel, PowerPoint, OpenDocument and text files in Go, one job at a
  time, at most two minutes and `UNDERDOC_TEXT_MAX_SIZE` each, three tries. An Office file is checked part by part
  against its packed size before anything is unpacked, so a zip bomb is refused. Images, and scanned PDF with
  `UNDERDOC_TESSERACT` set, go to the program tesseract (deu+eng), which the image of UnderDoc does not bring.
- **AI:** a model for OCR of difficult scans comes with M-13; the interface is there. A scanned PDF goes to it when the
  uploader has this way on, and the pages it reads count against a quota per user and per tenant and month.

| Setting | Meaning | Default |
|---|---|---|
| `UNDERDOC_TEXT_WAYS` | The ways the hoster allows: `browser`, `server`, `ai`, or `none` | `browser,server` |
| `UNDERDOC_TEXT_MAX_SIZE` | Largest file the server reads | `100M` |
| `UNDERDOC_TESSERACT` | The program tesseract for OCR on the server | none |
| `UNDERDOC_AI_OCR_PAGES_USER`, `UNDERDOC_AI_OCR_PAGES_TENANT` | Pages a model may read per month, `0` for no limit | `0` |

## Attachments of pages

Images pasted or dropped into the editor of a page are uploaded as attachments of the page and appear on a line of their
own, `![name](/api/pages/<page>/attachments/<id>)`. They belong to the page like its blocks and live only in the store.
The facts of the page list them. A copy of the page has copies of them. "Move to the files"
(`POST /api/pages/<page>/attachments/<id>/move` with `{"parent": "<folder>"}`) makes one a file in a folder, and every
block of the page shows the file from then on, as a new version of the block. The workspace moves it into the files
linked to the space of the page, or else into the user's own folder.

## Spaces and their files

A collection of the documents can name one root of the files as "files of this space": `PUT /api/spaces/<id>/files` with
`{"root": "<root>"}`, by the owner of the collection. Whoever sees both sees the link.

## Backup

A backup takes the database and the content store together. `underdoc export <dir>` writes, with `UNDERDOC_STORE_DIR`
set, the files below `files/` next to the documents:

```text
backup/
  files/
    .underdoc-files.yaml        roots, IDs, owners, every version with its hash, the attachments of pages
    Company/Contracts/plan.pdf  each file in its current version, under its folders
    .blobs/<sha256>             older versions and attachments, each once
```

`underdoc import [--tenant <id>] --files --owner <login> <dir>/files` brings them back: every copy of every content is checked against its
hash first, then everything comes in one transaction. IDs are kept where they are free. Owners and authors are found by
their login, else the owner given; a user's folder comes back as that user's folder if they have signed in once and have
none yet, else as a collection. Import the documents first, so attachments find their pages.
