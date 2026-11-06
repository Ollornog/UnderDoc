# UnderDoc concept

Status: proposal. Decisions are recorded in [`backlog/`](../backlog/), which wins if the two disagree.

## Goal

All knowledge lives in one place. Pages are Markdown, media is only embedded, and every fact has exactly one source.
People who want Excel keep Excel on a share, and the web shows the same file.

## Core idea: documents with an owner, folders only arrange them

A document has its own identity, roles and versions. The folder tree is a separate layer, so moving a document changes
where it appears, not what it is. See [ADR-13](../backlog/ADR-13-documents-and-folders.md).

- **Roles:** one owner (person or group), plus editors, commenters and viewers, per person or group.
- **Content stays unchanged:** Markdown is exact text, an Excel file is the exact file, and each version is kept under its hash.
- **The share is a projection:** chosen folders appear as real files. Changes from there become new versions.
- **Way out:** `underdoc export` writes everything as normal files at any time.

```mermaid
flowchart LR
    win[Windows, Excel, LibreOffice] <-- SMB --> share[Share: real files]
    share <-- watcher --> docs
    folder[Folder source, read-only phase] --> docs
    editor[Web editor, later] --> docs
    docs[(Documents, roles, versions)] --- tree[(Folder tree)]
    ext[Databases, Baserow, REST] --> cache[(Cache and index)]
    docs --> web[Web: wiki, notes, folders]
    tree --> web
    cache --> web
    docs --> public[Public sharing]
    docs --> out[API, exports, reports]
```

## Embedding: pages only reference their sources

| Object | Source of truth in the read-only phase | Embedding in Markdown (proposal) |
|---|---|---|
| Page | `.md` file with an `id:` in its front matter | `[[Page]]`, resolved by ID |
| File | File in the folder | `![[plan.pdf]]` |
| Excel table | Named table in an `.xlsx` | `::sheet{file="tower.xlsx" table="dimensions"}` |
| Snippet | File `_snippets/customers.sql` | `::snippet{id="customers"}` |
| Value | `_values.yaml`, optionally a named Excel cell | `{{tower.height}}` renders as “50 m” |
| Source, view | `_sources/*.yaml`, credentials only from the environment | `::view{id="open-tickets"}` |
| Native list | Database, with web editing | `::view{list="contacts" filter="customer=4711"}`, `{{customers:4711.phone}}` |
| Password | Encrypted store, never in Markdown | `::secret{id="router-admin"}` |

- **In the read-only phase everything comes from a folder source**, so a single source works without an editor.
- **Every object shows “used in”.** That makes a change predictable.
- **A missing value is a visible error** plus an entry in a list of broken references, never a silent gap.
- A view can take its SQL from a snippet, so documentation and table share one query.
- Embedding has a depth limit and cycle detection.
- Wiki, notes and folders are **one tree with three lenses**. “My notes” is a private library per user.

## Ideas taken from other tools

Concepts only. No code, text, translations or assets from these tools, see [ADR-1](../backlog/ADR-1-build-instead-of-fork.md).

| Idea | Seen in | In UnderDoc |
|---|---|---|
| Notebook, section, page | OneNote | Top folder, folder, page. No free placement on the page |
| Table of contents next to the page | Outline, Docmost | M-1 |
| Backlinks | Outline | M-1 |
| Search field on `/` and Ctrl+K | Outline | M-1, a command palette in M-12 |
| Recently changed | Outline | M-1 |
| Page history and restoring | Outline, Docmost, OneNote | M-2 |
| Public share links | Outline, Docmost | M-5 |
| Comments | Outline, Docmost | M-5 |
| Favorites, recently viewed | Outline | M-16 |
| Slash menu, templates | Outline, Docmost | M-8 |
| Quick notes without choosing a place | OneNote | M-8 |
| Diagrams in the page | Docmost | Mermaid in M-3, drawing in M-11 |
| Tags that can be searched, text in images | OneNote | M-12 |
| Collections above a tree that is mostly open | OneNote | M-14 |
| Documents side by side | Code editors | M-15 |
| Mentions with an inbox | Confluence | M-16 |
| Calendars in the wiki | Confluence | M-18, as a view of existing dates |
| Review, comments and issues on changes | Code forges | M-20, on the forge itself |

## Workspace

A small shell holds everything, and modules plug into it ([ADR-14](../backlog/ADR-14-one-topic-at-a-time.md)). Variants
are clicked through as mockups before any of this is built (T-5).

- **Tabs:** a sticky top bar holds the areas, for example Documents and Data. Favorites, Inbox and Calendar join when
  their modules exist.
- **Tree:** the left navigation stays, slimmer, with collections above it. Folders open by themselves up to a set number of children.
- **Windows:** 1 to 4 columns, each with its own document and table of contents. One document can be open twice.
- **Page width:** a document recommends slim, wide or full, where full hides the rail. The reader decides.
- **Facts panel:** name, location, who created it, who can see it, and what it uses: external and internal links,
  internal data, values, databases. It grows with each module.

## Two kinds of data

Some data can simply be changed: a save becomes the current version. Other data, such as code, needs someone to review and
confirm a change. That is a property of the folder. The starting recommendation is to use Git for it, so review and issues stay
on a forge instead of being rebuilt here (T-7).

## Order of work

One topic at a time, and each closes with its acceptance. Numbers are IDs, not an order.

1. **Finish M-1:** B-1, the gate in front, acceptance.
2. **M-14 workspace,** starting with the mockups (T-5). The shell comes first, so modules have something to plug into.
3. **M-15 windows.**
4. **M-2 files and Excel,** which brings versions, then **M-3 single source.**
5. **M-4 sources, M-5 permissions and sharing, M-16 favorites and inbox.**
6. **M-6 lists,** then **M-18 calendar** and **M-19 forms,** which build on them.
7. **M-7 output, M-17 import and export.**
8. **M-8 editor, M-9 inline editing, M-10 working together, M-20 reviewed documents and Git.**
9. **M-21 websites, M-11 drawing, M-12 search, M-13 AI.**

The order can change between topics, not during one. M-12 in particular can move forward once there is enough to search.

## Building blocks

| Part | Choice | Reason |
|---|---|---|
| Server | Go | One binary, strong standard library |
| Read-only UI | `html/template`, HTMX, C22 design system | No JS toolchain, strict CSP possible |
| Markdown | goldmark with custom directives | Extensible |
| Reading Excel | excelize | Reads stored results, calculates nothing itself |
| Database | Postgres | Native lists, full-text search, many writers |
| Sign-in | TinySesam as a gate in front | OIDC, SAML, LDAP, group mapping, independent of language |
| Editor (later) | TipTap, Yjs, Hocuspocus | Existing parts, check each package's license |
| Excel in the browser (optional, later) | Collabora via WOPI | Check its limits first |

## Excel and the share

- **Desktop Excel on SMB has no co-authoring.** One person writes, others read. Real parallel work needs native lists
  or the later browser route.
- **Editing starts in the folder.** Open the file in Excel or LibreOffice. The web shows its path with a copy button.
- **Office saves through renames.** The watcher waits for quiet, ignores temp and lock files (`~$…`, `.~lock.…#`),
  validates the ZIP before reading and shows the last good version on failure. The inode is no identity.
- **Moves in Explorer:** the same hash at a new path counts as a move of the same document, anything else as “orphaned”.
- **Addresses:** named tables and named cells are stable, `A1:D20` drifts.
- **Files not written by Excel** may lack stored formula results, which the UI marks.
- **Version history:** the server keeps a copy of every change, which a bare share lacks. See [Versions and audit](#versions-and-audit).
- **The watcher only sees local disks.** A local Samba share uses inotify, a remote file server uses polling.
- **Limit:** whoever cannot reach the share can only read and download until inline editing arrives.

## Lists

- **Every list has one source of truth:** native list, Excel file or external source, never two. Moving from Excel to
  native is a one-time import.
- **Field types:** text, number, date, choice, e-mail, phone, file, and **links to other lists**.
- **Web editing from the start**, as a deliberate exception to read-only first, using server-rendered forms and HTMX.
- **For Excel users:** a scheduled export to the share, marked as a read-only copy, with no way back.
- **Storage:** one JSONB record per row plus a link table, with history for every change.
- **Personal data:** permissions per list, audit log, deletion that includes history and exports, backup retention.
- **Deliberately not a CRM:** no sales pipeline, no mail campaigns.

## Diagrams

- **Mermaid** lives as text in the Markdown, served from our own origin.
- Values and snippets work inside diagrams: `{{router.ip}}` is replaced and escaped before rendering.
- For network topologies, `flowchart` with `subgraph` is the stable base, since `architecture-beta` is still in beta.
- **Open question:** Mermaid's inline styles may conflict with a strict CSP. To test: nonces, or rendering in an
  isolated frame. D2 is the fallback, as it renders on the server.
- **Later:** mind maps through Mermaid, drawing through Excalidraw and draw.io, stored as files in the folder.

## Passwords in the text

- **The value never enters Markdown.** It would otherwise sit in clear text on the share, in search, in versions,
  exports and backups. The text holds only the reference `::secret{id="…"}`.
- **Storage:** encrypted in the database with a key from the environment, using a maintained library such as age.
- **Separate permission per password.** Reading the page is not enough. The value arrives from the server on click,
  never in the page HTML, and every reveal is logged.
- **Never included** in search, exports, reports, public shares or the API.
- **Warning** when Markdown looks like it contains a password.
- **Honest limit:** whoever holds the server and the key can read everything. This is convenient operations
  documentation, not a vault. End-to-end encryption with groups is out of scope.

## Permissions

- **Roles belong to the document:** owner, editor, commenter, viewer. Folders pass default roles on to their contents. The
  default is no access.
- **A move changes inherited roles only**, and the UI says so first. Ownership can be transferred, and a group can be the owner.
- **Two doors, one model:** the web and SMB must agree. The app is the source of truth and derives Samba permissions
  from it. Owner and editor can write, commenter and viewer can read, and commenting only exists in the web.
- **Unproven:** whether per-file rights survive Office saving through renames (T-1). If not, share-enabled folders
  fall back to rights per folder, and the UI says so.
- **SMB cannot use OIDC.** Windows access needs a domain (AD, Kerberos) or a separate share password per user.
  Without a domain that means NTLMv2, which Microsoft lists as deprecated, although it keeps working for now.
- **Embedding bypasses nothing:** without permission on an embedded view, the reader sees “no access”.
- **Public sharing:** its own path with a token, optional expiry and password. All embedded objects are listed and
  released explicitly. Public pages only show frozen data, never a live query.
- **Sources:** queries run only on the server, with read-only accounts, time and row limits and a target allowlist.

## Versions and audit

Who changed what and when, on every door, from M-2 on. See [ADR-11](../backlog/ADR-11-versions-and-audit.md).

- **What and when:** every change is a stored version with hash, path, size and time. Deleted files can be restored.
- **Who:** the app knows the user for web, WebDAV and check-out. For SMB, Samba's `vfs_full_audit` logs user, IP, operation
  and file, and the app matches that to the version. Otherwise it says “user unknown” rather than guessing.
- **Native lists** keep a history per row.
- **Limits:** quick saves can merge into one version, and root on the server can alter the log.
- **Personal data:** retention is configurable, and employers must check their own rules before using it.

## Ways to reach the files

| Door | For whom | Limits |
|---|---|---|
| SMB with a password per user | Windows on a local network | Not for the internet, NTLMv2 without a domain |
| SMB in a domain | Companies that allow the join | Needs approval from their IT |
| WebDAV over HTTPS, served by the app | Outside the network, macOS, Linux, rclone | The Windows WebClient service is deprecated and off by default |
| Check-out in the browser | Anyone, without a client | Manual: download, edit, upload |

| Platform | Way | Note |
|---|---|---|
| Windows | SMB on the network, otherwise rclone with WebDAV | |
| Linux | SMB or WebDAV | |
| Android | Web, plus a file manager app with WebDAV or SMB | No offline sync |
| iOS | Web, plus an app with WebDAV. Check whether the Files app reaches SMB | No offline sync |

All doors end in the same permission model. See [ADR-10](../backlog/ADR-10-file-access-doors.md).

## Operation

One instance per customer, no tenants in the code. That avoids the most expensive class of bug, data crossing
customer boundaries. Web server and Samba on one host is the simplest setup.

## Search and AI, later

- **Search:** Pages, file contents, tables, lists and cached views, showing only what the reader may read. PostgreSQL
  first, a separate service only if measurements call for it ([M-12](../backlog/M-12-search.md)).
- **AI:** Optional and off by default, with a configurable and possibly self-hosted model. It only draws on what the asking
  user may read, names its sources and never sees passwords ([M-13](../backlog/M-13-ai.md)).

## Risks

1. **Scope:** this is a wiki, a list tool, a file server and a calendar in one, next to a code forge. Every milestone must be
   usable on its own, one is open at a time, and what a good tool already does stays there (issues, groupware, CRM).
2. **SMB identity** differs per environment.
3. **The share next to a real-time editor:** open sessions and outside changes collide. Solved in M-10, with a per-page lock until then.
4. **Markdown round trip:** the editor must not lose directives. Test before M-8.
5. **Keeping the share and the store in step:** the projection is the most delicate part of the model and needs tests for every failure.
