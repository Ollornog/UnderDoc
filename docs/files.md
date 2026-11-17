# Files

Files that are files stay files (ADR-13, ADR-19, ADR-28). The view **Files** shows them next to the documents, with
versions, a trash, quotas and uploads that resume. Samba and WebDAV come later (M-2, M-7); this page describes what is
built.

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

- **The folder of the tenant** ("Company"): made when an administrator first opens Files, and owned by them. Everyone
  reads it; the group named as its writers changes it, at first the group of administrators (`UNDERDOC_ADMIN_GROUP`).
- **My files:** a folder for each account, seen only by it.
- **Collections:** made by anyone, shared with everyone signed in, with the levels of ADR-20 (edit, comment, read).

Rights per group and per person come with ADR-29. The folder of the tenant and the folders of users are never deleted or
moved. Files and folders stay in the view Files, and pages stay among the documents: a move or copy between the two is
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
An older version comes back as a new version, so nothing is lost.

## What a file is, and how it is served

The type comes from the content, never from what the browser says: a PNG named `report.pdf` is a PNG. The name only
tells apart what the content cannot, such as the kinds of Office files, which are all zip archives, or Markdown from
plain text. Raster images, plain text, sound and video are shown in the browser; everything else is downloaded, and above
all HTML, SVG and XML, which can carry scripts. Everything served from the store has a policy under which nothing runs
(`sandbox`), and its type is never sniffed. Content is only served through a file or attachment the viewer may see.

## Attachments of pages

Images pasted into a page belong to it like its blocks and live only in the store
(`/api/pages/<page>/attachments/<id>`). A copy of the page has copies of them. "Move to the files"
(`POST /api/pages/<page>/attachments/<id>/move` with `{"parent": "<folder>"}`) makes one a file in a folder, and every
block of the page shows the file from then on, as a new version of the block.

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
