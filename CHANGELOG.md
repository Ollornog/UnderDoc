# Changelog

All notable changes to this project are documented in this file. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project uses
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

- **Rights per action with allow and deny, nested groups and the group Agents** (ADR-36, T-26,
  [docs/rights.md](docs/rights.md), migration 030). A right is now one of the actions read, comment, create, write,
  delete, share and manage, each inherit, allow or deny per person, group, the tenant or Agents; the roles of ADR-29 stay
  as presets, and entries that are none show as custom. For each action the nearest object with an entry for the account
  or one of its groups decides, deny beats allow on the same level, and without an entry the action is denied; the owner
  keeps every action on their own object, administrators manage rights whatever a deny says, private stays private.
  Local groups contain persons, groups and roles of the gate, cycles are refused by the database, a grant holds for the
  subgroups unless it says otherwise, and group admins manage their group and those inside it. Every request that did
  not sign in as a session belongs to the built-in group Agents (`Remote-Auth` of the gate, fail closed; a session of the
  sign-in pages is a session; `account.User.AuthKind` for T-27): a deny on Agents is a ceiling after the evaluation, for
  the owner too; agents never manage, share, delete for good or read passwords; a space can be set to no agents, and
  every person limits their own agents. A deny holds in the tree, the API, embedding ("not to be found"), the export and
  the search: the index keeps the levels of entries on read and the denies on Agents (search source 6, Meilisearch and
  OpenSearch settings 2, so every index is filled again once), and each hit is checked once more. Moving to another place
  needs delete where the object is and create where it goes, and never makes the mover someone who may share or manage
  it. In a library shared over Samba a deny on a single file is refused, like breaking its inheritance. The access check
  and the log (`refused`) name the deciding entry and where it comes from. Existing grants become allow entries of their
  preset, broken inheritance stays broken. API: `PUT /api/objects/<id>/grants` takes `actions` and `subgroups`, `GET
  /api/rights/check` answers each action (`&agent=1` for an agent), new `GET /api/groups/<id>`, `PUT
  /api/groups/<id>/groups/<group>`, `PUT /api/groups/<id>/admins/<account>`, `GET`/`PUT /api/me/agents`.

- Repokit 0.59.1 (with 0.58.0): `fc-pin` keeps one rolling pin PR `pin/underdoc` in the deployment repository and closes the old
  per-commit ones. Dependabot's updates of `deploy/compose.yml` (Mailpit, Gotenberg) now merge by themselves:
  `docker_compose` joined the ecosystems of the auto-merge, because the required check runs `scripts/check-services.sh`. New `nightly-repeat.yml` runs `ci.yml` (now with
  `workflow_call`) twice a night on `main`; a result that differs between the runs is red (flaky). The fast pre-push
  replaces "check.sh twice green" before each push.

### Added

- Acceptance test for Mailpit in `scripts/check-services.sh` ([docs/services.md](docs/services.md)): a mail sent over
  SMTP has to come out of `GET /api/v1/messages` with subject, recipient and text (`internal/mailpit`, tag `mailpit`).
  Until M-22 sends real mail it tests the service itself; with it Dependabot's updates of Mailpit merge by themselves
  like those of Meilisearch and Gotenberg. Mailpit 1.34.0.

- The table view of a spreadsheet in the workspace (ADR-31, [docs/tables.md](docs/tables.md#in-the-workspace)): the
  visible sheets as buttons, a head and row numbers that stay while scrolling, pages of 500 rows, a range marked with
  the mouse or the keyboard, Embed (the line to paste goes to the clipboard) and Download CSV for the range or the sheet,
  the names and Excel tables of the file with Embed, and what became of each embedded table in words. The PDF preview
  stays below; the facts panel names the pages that use the file; a hit of the search opens its sheet and rows.
- Search over tables (ADR-31, M-2, [docs/tables.md](docs/tables.md#search)): the cells of the visible sheets of a
  spreadsheet go to the search service, Meilisearch and OpenSearch alike, a record for each run of rows headed by its
  sheet; a hit opens `<sheet>!<range>`. Hidden sheets, rows and columns are not found, cell addresses are kept out of the
  text so they never match a code, and only readers of the file find it. The search source moves to version 5, so every
  index is filled again once. One cache of spreadsheets serves the web and the search.
- Tables of spreadsheets in pages (ADR-31, M-3, [docs/tables.md](docs/tables.md#tables-in-pages), migration 029): a
  sheet, a range, a defined name or an Excel table gets an ID local to its file when it is chosen (`POST
  /api/files/{id}/tables`), and `::embed{ref="<file>#x3"}` shows it live in a page, a name of one cell inside a paragraph
  as a value (`:embed{ref=…}`). A new version of the file is followed: names and tables by their name, a sheet renamed in
  Excel by its number, a range by its first row; a range whose first row changed shows a notice and is a problem until
  kept, and what is gone says so with the version, never quietly wrong. Who may not read the file sees neither cells nor
  its name; a read link shows a table only if its file was released and its maker still reads it. The file lists where
  its tables are used, the facts of a page name them, the list of problems names those that do not show, and the export
  of the files keeps their IDs.
- Spreadsheets as tables (ADR-31, [docs/tables.md](docs/tables.md)): UnderDoc reads `.xlsx`, `.xlsm`, `.ods` and `.csv`
  itself (`internal/sheet`, no new dependency) and answers the visible sheets, defined names and Excel tables of a file
  (`GET /api/files/{id}/sheets`), any range as it shows in the reader's language (`…/sheets/grid`, 500 rows a page) and
  as CSV without formula injection (`…/sheets/csv`). The stored results of formulas, number formats rendered from their
  codes (German or English by the browser's first language, held against what LibreOffice displays), merged cells,
  hidden sheets, rows and columns left out. Limits against zip bombs, entities, huge sheets and time; a file past them
  answers its reason (422); a page of a grid is at most 256 columns wide and a CSV at most a million cells, so a lone
  cell at XFD1048576 costs nothing. `.xlsm` is now recognised and previewed through the Office module like `.xlsx`.
- How spreadsheets become tables ([ADR-31](backlog/ADR-31-spreadsheets-as-tables.md)), before its code: a reader of
  UnderDoc's own for `.xlsx`/`.xlsm`, `.ods` and `.csv`; a sheet, a range, a defined name or an Excel table embedded by
  an ID local to its file (`<file>#x3`) and followed through new versions, with a notice where it cannot be followed;
  cached values, number formats in the reader's language, hidden stays hidden; rights, read links, export and search as
  for the file; limits against zip bombs, entities and huge sheets, and CSV without formula injection. Read only.
- The quota reaches the client of Samba (ADR-30, [docs/samba.md](docs/samba.md)): after each pass the companion gets the
  room of each shared library that a quota, the limit of the tenant or the storage of the hoster binds (the new request
  `room`), and smbd tells it through `dfree command`, so Windows shows it on the drive and Explorer refuses a copy that
  does not fit. A save over SMB that a quota or the largest size turns away is told to whoever saved it: once in their
  log, when the workspace opens, and in the profile under SMB share. **The companion changed:** its image must be built
  again; a companion from before keeps working without the room.
- A password typed as text (ADR-7, [docs/secrets.md](docs/secrets.md)): a line such as `Router password: hunter2`, or a
  row of a table with such a label, holds the page back in the editor before anything is sent, and the editor asks below
  the line: Keep it as a password makes a secret from the value and writes its reference in its place, It is not a
  password saves it as typed. Code, comments, placeholders and values already stored never ask; the first letters of a
  value being typed are not saved either.
- A link to read a page shows the images pasted into it and into the pages it released (ADR-29): they come through the
  address of the link (`/s/<token>/p/<page>/attachments/<id>`), only those the frozen page shows, and only while the
  maker of the link sees their page. A link of the page into the workspace becomes plain text, since whoever holds the
  link has no way in there; a link to a heading of the page itself stays.

- Passwords in the text (ADR-7, M-5, [docs/secrets.md](docs/secrets.md), migration 028): the text holds only
  `::secret{id="…"}`; the value is sealed by the vault (`UNDERDOC_SECRET_KEY`), each version on its own, and comes from
  the server on a click only (`POST /api/secrets/<id>/reveal`, same origin, `no-store`, 20 a minute per account), logged
  before it is answered. Whoever reads the page reveals and copies, or fewer that its managers name; a role lent by a
  share link does not count. A reference shows only on the page of its secret (embedded too), elsewhere "not available
  here", so copies, templates and imports take no values; moving a block takes its secrets along if the mover may reveal
  them. Never in the search, exports, the versions of a page, frozen links, the share or the logs.
- The editor makes passwords from the slash menu and the bar and shows them as masks with Reveal, Copy and Change; the
  page shows masks with Reveal and Copy; the settings of a page list its passwords with versions, readers and deletion.
- `underdoc rekey` with `UNDERDOC_SECRET_KEY_OLD`: every sealed value of every tenant (passwords of pages, TOTP, the
  client secret of OIDC, Samba hashes) sealed again under a new key; while the old key is set, both open.

- T-1, the part of LibreOffice ([backlog/T-1](backlog/T-1-measure-office-saving.md), `scripts/t1-libreoffice.sh`):
  LibreOffice 26.8 headless saves `.xlsx`, `.ods`, `.docx` and `.odt` over a real Samba share while smbd's audit records
  every operation. It saves into `lu<random>.tmp` and renames it over the file, so a right on a single file does not
  survive a save (rights stay per folder) and a file without read access stays hidden. The recordings are fixtures in
  `internal/share/testdata/t1`, replayed by `TestRecordedSaves` in every run.

- Samba (ADR-30, M-2, M-5, [docs/samba.md](docs/samba.md), migration 027): the folder of the tenant, each account's own
  folder and collections switched on by their managers appear as SMB shares, served by a companion container
  (`deploy/samba/Dockerfile`, Samba 4.24, an example in `deploy/samba/compose.yml`) that UnderDoc steers over a Unix socket
  without root (`UNDERDOC_SAMBA`, `UNDERDOC_SHARE_DIR`, `UNDERDOC_SAMBA_SERVER`). The working copy on disk carries each ID
  as `user.underdoc.id`; inotify and full passes take saves as versions by whoever saved (the save of Office through a
  temporary file and a rename included), new files and folders, renames and moves, and deletions into the trash; the web
  writes to disk and changes nothing while a file is open over SMB (`409`, and the facts panel says who has it). Roles
  become POSIX ACLs per folder; a folder someone may read without the folder above becomes a share of its own for them.
- App passwords for SMB in the profile (`/api/samba`): shown once, only the NT hash kept, sealed by the vault; a new one
  ends the old sessions. Administrators list and remove Samba users; managers switch the share of a collection in its
  rights dialog, which also says that Samba knows rights per folder only.
- `scripts/check-samba.sh` in the required check: the working copy against two real companions (two tenants) with
  `smbclient`.

- How Samba is built ([ADR-30](backlog/ADR-30-samba-share.md)), before its code: a companion container per tenant with
  smbd and a helper on a Unix socket, so UnderDoc never runs as root; an app password per account; a working copy on
  disk for shared libraries with the ID as an extended attribute and inotify; POSIX ACLs per folder from the roles.

- The log keeps an entry 365 days (ADR-29, [docs/rights.md](docs/rights.md), migration 026): per tenant, set by its
  administrators from 30 to 3650 days in the section Log of the tab Rights (`GET|PUT /api/log/keep`), the change logged.
  The job that empties the trash deletes older entries of every tenant.
- Administrators transfer what an account left behind: "what may they do" in the tab Rights lists what it owns by title,
  each with Transfer; the administrator sees nothing of the content, and the log keeps the transfer.

- The log of access and changes (ADR-29, M-5, [docs/rights.md](docs/rights.md), migration 024): opening and changing
  objects once per account, object and hour; roles, inheritance, owners, groups, members, administrators and links each
  time; share links opened, used, a wrong password and uploads through them. Nothing private is logged. Administrators read
  all of it in the tab Rights and in the administration of the profile, everyone else what they did and what happened to
  what they manage (`GET /api/log`).

- The rights in the workspace (ADR-29, M-5, [docs/rights.md](docs/rights.md)): the rights dialog of pages and
  collections (owner and transfer, inheritance, roles for everyone, groups and persons, share links with the embedded
  pages to release), owner and who sees a page in its facts panel, and the tab Rights in nav1 with groups, users, share
  links and the access check ("what may someone do", "who sees something"); Users and groups in the administration of the
  profile. Commenting without editing is a role of its own, on the whole page and on one block.
- Taking back an administrator: an administrator makes a local account an administrator or takes that back, never their
  own; whoever stops administering the tenant stops administering its organization in the time tracking where UnderDoc
  made them do so.

- Share links (ADR-29, M-5, [docs/rights.md](docs/rights.md)): whoever manages an object makes a link with a mode (read,
  upload only, comment, edit), an optional expiry and an optional password; only the hash of the token is kept. A page
  is shown frozen as it was when the link was made or renewed, with only the embedded pages its maker released; a file or
  a folder of the files shows its current files; upload only takes files under new names and shows nothing; a link to
  comment or edit gives its role to the accounts that open it while signed in. Links are listed with maker, expiry and
  users, and revoked (migration 023).

- Roles for persons, groups and the tenant (ADR-29, M-5, [docs/rights.md](docs/rights.md)): owner, manage, edit,
  comment and read on every object of the documents and of Files, inherited down the tree, narrowed by breaking the
  inheritance (which copies what held from above), ownership transferred with the former owner keeping edit. Local
  groups made by administrators, and groups taken over from OIDC and the gate. One function of the database decides the
  role, and every query that lists, finds or opens objects asks it, embeddings and search included; the index keeps
  readers as persons, groups and the tenant and is filled again once. Administrators manage rights, not content; private
  notes stay private. API under `/api/objects/<id>/rights|grants|inherit|owner`, `/api/groups` and `/api/people`.

- The decision on rights (ADR-29, M-5): roles owner, manage, edit, comment and read for persons, groups (local, or
  taken over from OIDC and the gate), the whole tenant and links; only allowing, narrower by breaking the inheritance;
  administrators manage rights, not content; the tree an object lives in decides, and shared folders set a limit (T-9).

- The module **Time tracking** (ADR-25, M-24, off by default, [docs/time-tracking.md](docs/time-tracking.md)):
  EightPerDay, embedded at a pinned version, runs below `/time/` with UnderDoc's sign-in and in the tenant of the
  request. Its tables come with the prefix `time_` from its own migrations in UnderDoc's flow and are held to the
  guard of ADR-23; the door "time tracking" is part of the separation test of the web. The tenant is an organization
  there, accounts are persons, and administrators of the tenant administer the organization; what a person tracks
  privately only they see. Time and Vacation of the private area open its pages in a window of the workspace, in a
  frame that a redraw of the workspace leaves as it was. `GET /api/time/absences` lists the user's absences for the
  calendar of M-18.

- The hoster renames a tenant, and deletes an archived one for good with its ID typed: its rows in every table, its
  addresses, its files and its search index ([docs/tenants.md](docs/tenants.md)).

- The Resources tab is real for values and snippets (M-3, [docs/parts.md](docs/parts.md)): `GET /api/snippets` and a
  count of uses on `GET /api/values`, both counted in pages the reader sees; databases and external sources say "(demo)".
- The facts panel of a page names what it embeds: values by name, parts, sections, blocks and pages (`embeds`).
- **Move to page** in the menu of a block, while reading and while editing.

- Files are found by their text (M-2, ADR-24, [docs/search.md](docs/search.md)): the text of the current version of a
  file, and of the attachments of a page, goes to the search service with where it came from, read by who may see the
  file. Triggers of the database (migration 020) queue the file or page when a text, a version or an attachment comes;
  a source that finds more raises its version, and every index is filled again once.
- The text of images in the browser (ADR-24, [docs/files.md](docs/files.md)): Tesseract.js with the language data `deu`
  and `eng`, built from the lockfile in `web/tesseract` and served by UnderDoc itself; only its worker may compile
  WebAssembly.
- `deploy/compose.yml` with the services next to UnderDoc ([docs/services.md](docs/services.md)): Meilisearch and
  Mailpit, OpenSearch and Gotenberg as profiles, every port on loopback, the key from `.env`, pinned by digest and moved
  by Dependabot; `scripts/check.sh` checks the file.
- `UNDERDOC_SEARCH_KEY`: the key of the search service from the environment, for a container that gets it from a `.env`
  file; with `UNDERDOC_SEARCH_KEY_FILE` as well, UnderDoc refuses to start.

- Modules (ADR-25, [docs/modules.md](docs/modules.md)): `UNDERDOC_MODULES` allows modules and sets their defaults, the
  hoster narrows them per tenant, the administrator of a tenant switches them (`/admin/modules`), and each user hides
  what is on (`/account/modules`). Off means no route, no entry and no job, and never deletes data. Documents and Files
  are the first modules; `module.All` is the registry the later ones join.
- The hoster administration (ADR-23, [docs/tenants.md](docs/tenants.md)) on `UNDERDOC_HOSTER_HOST`: make tenants with
  the invitation of their first administrator, lock, archive and restore them, set limits on users and storage (pages and
  files together), the modules they may use and domains of their own. It sees counts, never content. Limits hold for
  every way an account, a block or a file comes in.

### Changed

- OpenSearch 3.10.0 in the tests, the CI and `deploy/compose.yml` (replaces Dependabot's pull request, which only moved the
  Dockerfile). The shared search tests and the German corpus give the same numbers as on 3.9.0, so the table in
  [docs/search.md](docs/search.md) only changes its version. The release notes list no change that touches the index
  (removed: the experimental Views, the macOS x64 archives).
- Meilisearch 1.57.0 in the tests, the CI and `deploy/compose.yml`. The Compose file sets `MEILI_UPGRADE_DB`, so a new
  image takes over the database of the old one instead of refusing to start; the upgrade is not atomic, so
  [docs/services.md](docs/services.md#updating-meilisearch) says how to take a snapshot first. The search engines of
  `deploy/compose.yml` now follow the pins of `scripts/search` like the services of the CI (`tests/test_repo.py`), and
  Dependabot moves them only there, so one update changes every place. `scripts/check-services.sh`, a new step of the
  CI, starts the services from the Compose file: Meilisearch has to take over a filled database of the version before,
  and Gotenberg has to convert a real spreadsheet through the client of the preview (tag `office`). A test proves that
  an index the search service lost is filled again although the database records it as built.
- C22 vendored at `6d8a2ae` (C22 #155), the last two bridges gone
  ([T-12](backlog/T-12-workspace-from-c22-building-blocks.md), [docs/editor.md](docs/editor.md#built-from-c22)): a
  heading in the editor has the room it has when read from C22, `editor.css` keeps no bridge; the list of the share
  combobox (rights dialog, new collection) leaves a section that scrolls (`data-popover-fixed`, upwards when there is no
  room below), so the steps of a new collection scroll within the dialog again; the drawn table in the editor stands in
  the box and frame it has when read; typing into the combobox no longer leaves the lines of empty groups.
- C22 vendored at `a023d57`, its old app workspace removed there (C22 T-20), and the last bridges of the switch gone
  ([T-12](backlog/T-12-workspace-from-c22-building-blocks.md), done, [docs/editor.md](docs/editor.md#built-from-c22)):
  the live preview of the editor says what a line, a mark or a widget is by C22's `data-md` and takes its look from C22,
  so `editor.css` keeps no look of its own, only what CodeMirror's parts need to sit in `.editor` (each rule says why);
  headings in the editor now have the sizes of the page when read (the pack's `--editor-h1`…`-h4`), a quote stands in
  with a bar and is italic as when read, inline code and a code block look as when read, and a Markdown sign left
  visible on the cursor's line is muted by C22; until C22 has it, a bridge in `editor.css` keeps the room above and below
  a heading as when read. The syntax colouring of CodeMirror (`@lezer/highlight`) went with it: no
  rule used it. Choosing whom to share with, in the rights dialog and when making a collection, is C22's grouped
  combobox, filtered by typing, each entry with the icon of its kind: the workspace loads Basecoat's core and its
  combobox alone, which take over nothing the workspace draws (a test proves it). The row of a spreadsheet a hit named
  is C22's current row (`aria-current`), no longer drawn as a selected one.
- Dialogs, the pages outside the workspace and the table view of a spreadsheet from C22's building blocks, and the root
  class of C22's old app workspace is gone ([T-12](backlog/T-12-workspace-from-c22-building-blocks.md), steps 5 and 6,
  [docs/editor.md](docs/editor.md#dialogs-and-pages-outside-the-workspace)): UnderDoc emits no class of the old block any
  more, and the class check finds every class the server writes as well (an embedding, a mask, a table) and fails on a
  class of the old block. The rights dialog shows owner and inheritance as entries with their action, each grant with the
  icon of its kind and a cross to take it back, and a role from above as a split badge (origin, role); a new collection is
  made in two steps, its name and then Share with (Enter in the name still makes it at once, shared with nobody); the
  passwords of a page are entries; the log in the tab Rights is a timeline; a dialog scrolls within the height of the
  screen instead of 420px. The table view shows the sheets as tabs and the sheet as C22's grid: a marked range shows on
  the letters and numbers of its columns and rows too, the cell it began at is framed, and numbers stand at the end of
  their cells (they never did: the script read the kind of a cell as a letter, while it comes as a number). Tables in
  pages mark numbers the same way, and the page behind a share link is C22's public page (its frozen page typeset, its
  files in a table that wraps). The workspace loads `c22.js` for the steps of the dialog; `data-behaviour="app"` keeps it
  out of the workspace, which a new test proves with a spy on every change c22.js makes. The values of a saved search are
  disabled until they can be changed (M-8), so c22.js does not toggle them. Without the root class, what the old block
  set for everything below it is gone: a link in a page is underlined and a table head plain as C22 typesets them, and
  the quiet lines of nav3 take the size of its hints. Choosing whom to share with stays a `select`
  with groups: C22's grouped combobox needs Basecoat's script, which the workspace does not load.
- C22 vendored at `bad252a` ([T-12](backlog/T-12-workspace-from-c22-building-blocks.md), after step 4): the looks the
  menus and views lacked come from C22 now, so their bridges are gone. The current hit of Find on the page is C22's
  `mark[aria-current]` (framed and bold), the element whose comments are open `.comment-anchor[data-comment-focus]` (for
  a table its wrapper, which is the anchor of its bubble), the icon in the head of a drawer has its size from the drawer,
  context menus have the pack's 13px rows, the drawer begins below the head of the window, and a module that is off says
  `✗ Off` beside the one that is on.
- Menus, drawers, panels and the other views of the workspace from C22's building blocks
  ([T-12](backlog/T-12-workspace-from-c22-building-blocks.md), step 4, C22 at `ec39f36`,
  [docs/editor.md](docs/editor.md#the-workspace-around-it)): right-click menus and the menus of the tab row are C22's
  `.context-menu`, More a `.dropdown-menu`, New in nav2 and in a collection a `.command`, New of the top row the popover of
  its button, the quick search a `.command`, a message `.toaster > .toast`; the gear, the comments and the assistant are the
  window's `aside[data-drawer]` (settings as `.button-group`, facts as `dl`, comments as `ol.timeline`, the assistant's
  bubbles in a log); profile, settings, administration and the tab Rights are a `.card` in nav3 with `.item` lists; mail
  and news are `.item` entries, to-dos `label.item` with a checkbox (ticked on change), the month `table.month-sheet`, a
  board `.board`, a saved search a `.filter-bar` with hits as `.item`s; content without blocks uses `.alert`,
  `table.table`, `code`, `.empty[data-dropzone]` and `figure.card`, so the class `doc` is gone. Every menu and popup hangs
  inside the `.workspace`. What shows differently: menus are the pack's (16px icons, a heading line), the drawer lies over
  the head of the window as C22's does, the clock-in is an outline button that stays pressed, the general tab no longer
  vanishes from the tab row while it is dragged, and a toast does not slide in again with every redraw. Two looks C22 does
  not have yet are bridged with its utilities (`bridge until C22` in the code): the current hit of Find on the page and the
  element whose comments are open.
- The editor and its parts from C22's building blocks ([T-12](backlog/T-12-workspace-from-c22-building-blocks.md), step
  3, C22 at `ec39f36`, [docs/editor.md](docs/editor.md#built-from-c22)): the page in the editor is C22's `.editor`, each
  block while reading a `div[data-block]` in the typeset flow with its handle `.editor-handle` (the class `doc` of the
  old block stays only on content without blocks), the slash menu `.command`, questions in place `.item`, a block used
  elsewhere `[data-used]`, a part `[data-part]` with its frame and name, an embedding `figure.editor-embed` (in the
  editor a badge), what happens where something in use goes a card over the workspace, quick notes a grid of cards,
  comment bubbles `.comment-anchor`, the source view `pre.code-block` with line numbers. What shows differently: the
  menu of a handle is the pack's menu (a little wider rows, 16px icons), the name of a part stands above it, a block
  used elsewhere has one frame round its lines while the cursor is in it, a value that looks like a password is
  underlined in waves by C22's utilities, a line or a space between entries is C22's `hr`, a collection kept open shows
  the pack's thick line instead of a pin, and a card of the quick notes has no speech bubble beside it (its handle has
  Comment with the count). What C22 lacked for the editor it got along the way (C22 #144, #145), so UnderDoc keeps no
  look of its own for it. A page frozen for a link before keeps its old markup, and its tables are still taken out when
  its maker no longer reads the file.
- The frame of the workspace from C22's building blocks ([T-12](backlog/T-12-workspace-from-c22-building-blocks.md),
  step 2): the top row, nav1 to nav3 with their lists, the windows with their tabs, the head of a window (path, tags,
  what others may do, tools, title, the button for reading or editing), the document column and the status line are
  C22's `.workspace` and its parts, the look the pack `underdoc` (C22 at `4bc20af`). What shows differently: the path
  separates its parts with a chevron, the tags are square chips, the sections of a page open as a menu under a small
  button, the find on a page is one field with its buttons, the entry open in the window says "open" in the list, a tab
  being dragged leaves a framed place and the window shows where it lands ("New column", "Move here", "Open here"), a
  collection kept open carries a pin, and a single window without tabs has no tab row (on a phone the row stays for
  the button "Window and tabs", which sits beside New elsewhere). The phone form starts below 48rem (was 760px), where
  nav2 and nav3 open over the window; there the tools of a window are the small size, and the status line shows the
  words and the format. Menus,
  drawers, the editor and the other views keep their old form until the next steps; drawers now lie over the whole
  window, its head too, as C22's do.
- Every page takes C22's layout pack `underdoc` (`c22-underdoc.css`, C22 at `6cff0e0`) instead of `vega`: UnderDoc's own
  look on C22's building blocks, the first step of moving the workspace off C22's old app workspace
  ([T-12](backlog/T-12-workspace-from-c22-building-blocks.md)). Corners are a notch tighter and prose leads a little
  closer on the sign-in, share and message pages; the workspace itself does not change yet. The class check now also
  reads every class the scripts of the workspace and the editor write.
- A file or folder of the view Files is made and renamed only with a name a file system takes (not `.` or `..`, no `/`,
  `\` or control characters), as an upload already required; one from before stays off the share. In a library shared over Samba a single file no longer breaks the inheritance (ADR-29).

- **A new collection at the top belongs to its maker alone** until they share it (ADR-29, decided on 2026-11-19,
  [docs/rights.md](docs/rights.md)): it grants nobody a role, not even the whole tenant, and is still not private. Its
  dialog asks for the name and has the step "Share with" (everyone, groups, persons, each with a role; by default
  nobody), granted through the API of the rights dialog. The same for a new collection of Files and a copy made at the
  top. Collections made before keep what they granted. The level "" at the top (`/access`) now means nobody instead of
  edit, and the overview of a collection at the top offers "Nobody".

- The time tracking runs EightPerDay `50e5120` (was `f388147`, EightPerDay T-17 and T-18): notes, projects, the
  details of a day and its incomes in an approved week change only through a change request with a reason, a note of a
  managed employer says who sees it, and the core refuses to write the details or incomes of a locked day
  (`core.ErrLocked`), so UnderDoc's store refuses them too and the page tells why
  ([docs/time-tracking.md](docs/time-tracking.md)). No migration; the records carry new JSON fields.
- The time tracking runs EightPerDay `f388147` (was `de9f1ea`, EightPerDay M-6): the start page is one sheet in three
  views (table, journal, a grid per week) with a level of detail, each field saving itself, and the view kept per person.
  Its list and every week are gone, so the list of Time in nav3 names the sheet instead
  ([docs/time-tracking.md](docs/time-tracking.md)). No migration of its own changed; the embedding stays as it was.
- The time tracking runs EightPerDay `de9f1ea` (was `338593b`): hours of half a day (`4v`, `4n`) and half a day of
  leave next to work, pay for half days, and standby with its pay and as working time
  ([docs/time-tracking.md](docs/time-tracking.md)). The embedding stays as it was.
- The access log names the address of a share link or an invitation without its token.

- The level for all others of ADR-20 is now the role of the whole tenant (migration 022): a narrower level below breaks
  the inheritance; the writers of the folder of the tenant are a group with the role edit. The owner of an object now
  manages what lies below it. The export writes `inherit: false` and `access: none` where they apply.

- The pages of signing in and of the administration use the building blocks of C22 0508dbf: the sign-in is C22's
  card (the card is the form of the password, other ways above it set apart by "or", registration as its action), a
  link that works once sits in C22's field with a button to copy it, the hoster sees what a tenant uses against its
  limits as a bar (`progress.progress`, ⚠ nearly full and ✗ full also in words), and the log of sign-ins marks its
  results as badges.
- The storage a tenant uses is a counter that triggers keep (migration 021), no longer summed over every block and file
  on each write; what each tenant used before is counted once by the migration.
- The CI runs Meilisearch and OpenSearch as services of its job (also started by ci-local); the hygiene test holds their
  images to the pins of `scripts/search/Dockerfile.*`.
- C22 vendored from 0508dbf (C22 #126, #127), which takes over the rules UnderDoc had kept
  "until C22 has them" (ADR-22): the question before something in use goes (`.wayspop`, `.wayitem`, `.wayrow`,
  `.waysame`), a part in running text and its boundary while reading and in the editor (`.partedge`, `.partlabel`,
  `.cm-ud-part`), and the note where something embedded was removed, with its chips. They are gone from the editor's
  stylesheet. The preview of a file sits in C22's frame (`figure.preview`), an upload shows C22's bar
  (`progress.progress`), and the place a search jumped to is C22's current mark (`mark.cur`). The class check now also
  covers these classes, which the script and the editor draw.

### Fixed

- Browser tests that failed on a loaded host and passed alone: `TestEveryPageIsClean` read `document.fonts.check` once
  after the page was drawn, but a font loads when the first text needs it, so it now waits for it; the drag tests waited
  a fixed 500 ms before reloading and expected the order or the access level already stored, so they now wait for the
  requests of the page to be answered (`settled`) and for the order to show. `UNDERDOC_BROWSERTEST_LATENCY_MS` and
  `UNDERDOC_BROWSERTEST_CPU_SLOWDOWN` slow the browser down to try this; with a latency of 250 ms the old tests failed and
  the new ones pass. `TestTheWaysOfAPartInUse` could not be made to fail again, so its failure message now carries the
  status history of the editor.
- A password typed as text whose value is emphasis to Markdown (`pw-_x_`): the editor hid the marks inside the value
  while it asked about it, so the question showed `pw-x` in italics while Keep it as a password seals `pw-_x_`. A value
  the editor asks about now shows as typed; Markdown that reaches into it is neither hidden nor drawn. This, not a lost
  key or a redrawn workspace, was the flaky `TestAPasswordTypedAsText` in CI ("the value is not marked", 18 of 20
  characters): one random value in 64 started with `_`. The test now types that shape every time and checks that
  nothing of the value stays behind its reference.
- A password typed as text: once the start of a value was long enough, the editor asked about it, and letters typed
  after it did not end the question until the next save. Meanwhile only the start was underlined, and Keep it as a
  password would have made a secret of the start and left the rest as text in the page. A question now goes as soon as
  its value is no longer the whole value of its line, and the next save asks about the whole value. Seen as a flaky
  browser test in CI (`TestAPasswordTypedAsText`, all but the last two letters marked); the test now pauses before the
  last letters and checks that the question about the start goes at once.
- The scripts that run test containers (`scripts/search-engines.sh`, `check-image.sh`, `check-samba.sh`,
  `t1-libreoffice.sh`) removed them without their anonymous volumes, so every run left some behind. They are removed
  with `-v` now, and `tests/test_repo.py` holds every removal in `scripts/` to it.
- The OpenSearch of the tests (`scripts/search-engines.sh`, the service in CI) no longer depends on how full the disk of
  the host is: above 90 % its disk watermark blocked creating an index and the search tests failed with
  `index_create_block_exception`. It runs with `cluster.routing.allocation.disk.threshold_enabled=false`;
  `deploy/compose.yml` keeps the protection, and `tests/test_repo.py` holds both.
- A test run that was killed (a timeout, an aborted suite) left its PostgreSQL running and its folder behind
  (`internal/db/pgtest`), for days and gigabytes. Every folder now names its test binary, and the next start stops the
  server of a folder whose binary is gone and removes it; only its own folders, never a running run's or another
  program's.
- The browser tests of the search waited for a change to reach the index only as long as for the workspace to draw (five
  seconds), though it passes the queue, the indexer, the engine and a search; a stall of the engine under load made
  `TestQuickSearchJumpsToTheSection` fail. They now wait up to 30 seconds for the search to find it, and say what it
  answered last.
- A password typed as text that ended in `_` or `*` (`hunter2_`) lost that character: the editor took it for the close of
  an emphasis, made a secret without it and left it in the page. Marks of emphasis are now dropped only when they are
  Markdown: a pair around the value, the end of a bold label, or the close of a bold opened before the label (ADR-7).
- A save over SMB that a quota turned away was written over by the database's content in the same pass, so it was lost
  instead of staying on disk without a version as ADR-30 says; now it stays, and each pass takes it up again (ADR-30).
- Taking a folder into something private now waits for whatever is being made or handed over at any depth below it, not
  only directly in it, so a page of someone else made at that moment deep inside never goes private with it (ADR-29).
- The form to transfer what someone left behind chooses nobody until the administrator does, and does not offer the
  account whose things they are; before, it chose the first account of the list, and sending it as it was gave the
  object to that account.
- The table of building blocks in the concept still named TipTap as the editor; it is CodeMirror 6 (ADR-21).
- The limit of 20 reveals of passwords a minute per account is counted in the log of the database, under a lock per
  account, so several instances behind one address no longer multiply it and a restart no longer resets it (ADR-7).
- A page that went into the trash while one of its parts got a new master brought the part back a second time when it
  was restored: the part now leaves the page in the trash, and the version says where it went.
- A block held at the edge of a page scrolled it by the frames the browser drew, so a busy browser scrolled less in the
  same time and the block landed short: it now scrolls by the time held.

### Security

- Spreadsheets as tables, attack round (ADR-31, [docs/tables.md](docs/tables.md#limits)): small files could exhaust
  memory or CPU after reading, in the reader, the table view, embeddings, CSV downloads and the search indexer. The text
  of the cells of a workbook now counts as often as it stands (100 MiB; a shared string or a repeated cell of a few
  bytes could become gigabytes once written out); a format shows a cell's text at most 32 767 characters long; hidden
  rows and columns are kept once and only up to the content (2 097 152 in a workbook); a page of a grid cuts merged areas
  at its edges and stops at 8 MiB of text; finding the first row of a range, writing a CSV and building the search text
  go through the cells once; the directory of a zip is counted before Go's reader loads it (for Office text too). The
  one sheet of a CSV is named after its own file when another file has the same bytes. What was read is kept as
  `-sheet-2`, so every workbook is read once more.
- Spreadsheets as tables, attack round (ADR-31, [docs/tables.md](docs/tables.md#csv-without-formulas)): a CSV download
  quotes every field, since German Excel and LibreOffice split at the semicolon and a text such as `x;=1+1` written bare
  became a formula there. A file keeps at most 10 000 tables with an ID and an anchor at most 256 texts of 256
  characters, so a reader who chooses tables cannot fill the database. An OpenDocument sheet hidden by a common style of
  `styles.xml` stays hidden.

- Samba, known limit ([docs/samba.md](docs/samba.md), ADR-30): a user who reaches a folder only through passage on the
  way to their own share can confirm a guessed name there, not list it or read it. Not fixed on purpose (it would need
  `CAP_SYS_ADMIN` in the companion); the test holds that listing and reading stay closed.
- Samba after an attack round (ADR-30, [docs/samba.md](docs/samba.md)): a change on the share whose writer neither the
  audit nor a Samba user owning the file names is no longer taken as the owner of the object but put back (something new
  stays on disk untaken), and the log says so; the audit keeps only operations that change something, numbered, so a
  flood of listings pushes nothing out and a gap is logged. Hard links made over SMB2 end at the next pass instead of
  handing an object's ID to a second name. A file moved to another folder or saved through a rename gets the ACL of its
  folder. Sharing a collection that holds a file breaking the inheritance, or moving one into a shared library, is
  refused naming the files, and one that is there anyway stays off the disk. A library that stops being shared (or the
  folder of a user whose app password went) gets a last pass first and stays on disk while something there is not taken.
  A Samba user may not take the name of a share. Folders named like temporary files (`*.tmp`, `._*`, `~$*`) are ordinary
  folders, and no share path holds `%`. The companion changed (`cmd/underdoc-samba`): its image must be built again.
- Passwords in the text (ADR-7, [docs/secrets.md](docs/secrets.md)): a block moved with its secret is logged on the page
  the secret leaves too, so a move into private notes leaves a trace.
- The rights of ADR-29 after an attack round ([docs/rights.md](docs/rights.md)): moving an object to another place
  needs manage on it (reordering within its parent stays with edit), so whoever only edits no longer becomes its manager
  by moving it into a collection of their own; only what is wholly one's own goes into something private, and its grants
  and links end there, logged; deleting and purging refuse when something below breaks the inheritance and shuts the
  deleter out; versions of pages and files are for edit; a link to read a folder shows only what its maker sees now,
  object by object; an upload through a link takes its name when the file is made and never becomes a version; search
  paths show "…" for levels not every reader sees (the index is filled again once); "what may X do" names nothing
  private; taking back administrators is decided under a lock in the database, so two at once never leave a tenant
  without one. Migration 025 gives the group of writers edit again in the narrower folders of the folder of the tenant,
  as under ADR-20.

### Added

- Sign-in built in (ADR-23, [docs/sign-in.md](docs/sign-in.md)), chosen per tenant: local accounts with a passkey
  (WebAuthn, the default), with a password and a TOTP code, or with a password alone; OIDC with any provider (PKCE,
  state, nonce; the account named by `preferred_username`, groups from a claim); and the gate in front. Registration is
  off by default; administrators invite with a link that works once, also from `underdoc invite`. Passwords as Argon2id,
  a limit against guessing, sessions as `__Host-` cookies (`HttpOnly`, `Secure`, `SameSite=Lax`) whose hash the database
  keeps, every sign-in logged. Pages: sign in, invitation, register, "Sign-in and security" of an account, and
  "Sign-in" of the administration.
- `UNDERDOC_GATE` decides whether the headers of a gate may count: an instance that ran behind a gate keeps it, unchanged,
  a new one starts with passkeys, and only whoever runs the instance lets tenants switch a gate on.
- A vault for secrets of other systems (package `vault`): AES-256-GCM under `UNDERDOC_SECRET_KEY`, bound to tenant and
  name, written and used but never shown or logged. It holds TOTP secrets and the client secret of OIDC.

### Added

- Parts and sections (ADR-22, M-3, [docs/parts.md](docs/parts.md)): a few words inside a paragraph are a part, marked by
  two point markers in the Markdown (`<!-- part t2 -->…<!-- /part t2 -->`, with `name="…"` a value) and addressed as
  `<page>#<block>.<part>`; a block that opens with a heading opens a section (`<page>#<block>.section`) up to the next
  heading of its level, and it grows with the original. Both are embedded with `::embed{ref=…}`, and a part also inside a
  paragraph with `:embed{ref=…}`. A part may cut through formatting: it is cut out of the parsed paragraph, with formatting
  open at its edges closed and opened again (table test and fuzz test). Marking (`POST …/blocks/{block}/parts`) snaps to
  atomic elements and stays in its paragraph; a copy (`POST …/blocks/{block}/excerpt`) is plain Markdown without markers;
  `GET /api/objects/{id}/uses?ref=` names every place of use. The list of problems names parts and sections that are gone,
  the export keeps the markers, and an import relinks inline embeddings too.
- Deleting something in use asks for its way (ADR-22, [docs/parts.md](docs/parts.md)): a block, the text of a part or a
  page of which something is embedded elsewhere. A dialog of the workspace names each thing with every place of use, with
  nothing chosen in advance: delete everywhere (each use shows who removed it and when, and Restore brings every use
  back), a copy in every use the viewer may change (others keep a placeholder, their owners a notice and "Keep a copy
  here"), or a new master among the uses, into which the content moves while every other reference is rewritten in the
  same transaction and the old address redirects. The server refuses such a change without a way (409). Moving a block
  into another page (`POST …/move {"page"}`) rewrites references and redirects the same way. New tables
  `removed`, `redirects` and `notices` with their tenant and row-level security; a version says where a copy came from.
- A part embedded inside a paragraph reads as the text around it, with a faint dotted line under it.
- Parts in the editor and while reading (ADR-22): with text marked, the bar copies it as plain text, makes it a part and
  copies its address, or makes it a value with a name; the address of a part pasted into a paragraph embeds it there.
  The markers of parts are hidden and atomic and never go on their own; the cursor in a part, the pointer over it while
  reading, or a finger on it shows its boundary and "Used in N places". `GET /api/values` lists the values.
- Tenants (ADR-23, [docs/tenants.md](docs/tenants.md)): every table carries its tenant, and PostgreSQL row-level
  security keeps tenants apart; a query without a tenant sees nothing. `UNDERDOC_TENANTS=one` (the default) keeps
  everything in the tenant Main, where the existing data moves; `many` finds the tenant by the host name, a subdomain of
  `UNDERDOC_TENANT_DOMAIN` or a domain of its own. A locked tenant answers 403, an unknown host 404. The folder source
  belongs to Main. `underdoc tenant add`, and `--tenant` for export and import. A connection as superuser steps down to
  the role `underdoc_app`; one with `BYPASSRLS` stops the start. The trash empties itself per tenant.
- A guard test fails for any table, also a later one, without tenant column, forced row-level security, a tenant-only
  policy, the tenant in its key and in every reference; a separation test goes through every door of the web.

### Changed

- C22 vendored from 572a276 (C22 #123), which takes over the looks M-8 had kept for itself: the outline and label of a
  block used elsewhere (`.cm-ud-used*`), the table tools as a group of the bar (`.btool .bgroup`, which wraps on a phone
  instead of scrolling), and the templates with what each is for, under its name in New at a collection and as cards in
  New in the top bar (`.tplcards`). The editor bundle drops its own rules for them.

- Every image published from main now also opens the pin PR in the fleet repo (job `fc-pin` in `image.yml`, kit script
  `scripts/_release_automatik.py`, target from the variable `PIN_REPO`): the new `main-<sha>` with its digest replaces the
  pin there; the fleet repo's start probe and auto-merge take it from there.
- Dependabot updates of `docker/setup-buildx-action`, `docker/login-action` and `docker/build-push-action` merge by
  themselves: they run only in `image.yml` on `main`, which now accepts them after the merge
  (`NACH_MERGE_ABGENOMMEN`). If that run fails, no new image is published. #50 had been waiting for a manual merge.

### Added

- Images pasted or dropped into the editor become attachments of the page (ADR-28): uploaded in chunks, shown on a line
  of their own in the editor and in reading, listed in the facts of the page, where "Move to the files" makes one a file
  in the files of the space or the user's own folder, and the page shows the file from then on. Blocks show images of
  the content store by their addresses (`/api/pages/<id>/attachments/<a>`, `/api/files/<id>/content`).
- The text of files for the search (ADR-24, docs/files.md), three ways, each allowed by the hoster
  (`UNDERDOC_TEXT_WAYS`) and switched by the tenant and each user: the uploader's browser reads PDF with pdf.js, a queue
  on the server reads PDF, Office and text files in Go (zip bombs refused before unpacking, limits on size, time and
  tries) and images with Tesseract where it is installed (`UNDERDOC_TESSERACT`), and a model for OCR of scans plugs in
  with M-13, with a quota of pages per user and tenant. Every text is kept with where it came from.
- Previews of files (ADR-28, docs/files.md): images get a small picture made on the server with Go alone (no decoding of
  images with more than 50 million pixels), PDF shows in the window through pdf.js, built from its lockfile in
  `web/pdfjs` and served by UnderDoc itself (Apache-2.0, licences beside it), and Office files show the same way when the
  optional Office module, a Gotenberg service at `UNDERDOC_OFFICE_URL`, converts them to PDF.
- Files of M-2 without Samba (ADR-28, docs/files.md): a content store keeps every version of a file and every attachment
  of a page as a blob named by its SHA-256, once per content and apart per tenant (`UNDERDOC_STORE_DIR`). The view Files
  shows its roots in nav2, the folder of the company, the user's own folder and collections (addendum to ADR-28), and one
  folder at a time in nav3, where files are uploaded by a button or by dropping them from the desktop; a file opens in
  the window with a preview of images and video, its versions with download and restore, and the trash. Uploads come in
  chunks of 8 MiB and resume after a break; their declared size counts against the quota of the user's folder, the
  collection and the tenant from the start. A file uploaded again under its name becomes a new version, and versions are
  thinned after 30 days to one a day and after 90 days to one a week (configurable). The type of a file comes from its
  content; only raster images, text, sound and video show in the browser, everything else is downloaded, and nothing
  served from the store runs. Pasted images are attachments of their page and can be moved to the files; a collection of
  the documents can name the files of its space. `underdoc export` writes the files with every version next to the
  documents, and `underdoc import --files` brings them back, each content checked against its hash.
- Quick notes are a document type (ADR-16, M-8): a board of small cards in the private notes, newest first, with a field
  on top to add one; each card is a block of the board. New with Quick notes chosen puts a card there without choosing a
  place first, New with a collection makes a page there, and the last choice stays with the account. See docs/editor.md.
- Templates start new pages (M-8): New at a collection and New in the top bar offer UnderDoc's own (Meeting notes, How-to,
  Runbook, Decision) and every page offered as a template from its gear; the page starts with the template's blocks and
  opens in the editor. A template of one's own stays where it was written, with its owner and access. See docs/editor.md.
- Blocks of a page are aligned left, center or right from the menu of their handle, in reading and in the editor (ADR-16,
  M-8). The alignment is kept with the block, not in its Markdown (`PUT /api/pages/{id}/blocks/{block}/align`), makes no
  version, goes along with a copy, and the export writes it into the block's marker (`<!-- block b3 align=center -->`),
  which an import reads back.
- The gear of a document of the database keeps its width (slim, wide, full) and where its contents show (marker, side,
  hidden) for everyone who opens it (ADR-16, M-8): `PUT /api/objects/{id}/view`, kept in the database, carried to the
  workspace with the tree, kept by a copy and written into the front matter of the export (`width`, `contents`), which
  an import reads back. The contents of such a page are its blocks that start with a heading. Who may only read or
  comment sees the choices greyed.
- A block used elsewhere shows its boundary and "Used in N places" while the cursor is in it in the editor (ADR-22), so
  whoever writes there knows that other pages change too.
- Table tools in the bar of the editor (M-8): while the cursor stands in a table, rows and columns go in above, below,
  left or right of it, a row or a column goes, and a column is aligned left, center or right. Each tool writes the table
  again as plain Markdown, and the cursor goes to the cell it names. See docs/editor.md.
- OpenSearch 3.9.0 as the second search engine (ADR-24), chosen with `UNDERDOC_SEARCH_ENGINE=opensearch`, with basic
  authentication (`UNDERDOC_SEARCH_USER`, the password in `UNDERDOC_SEARCH_KEY_FILE`). It passes the same tests as
  Meilisearch: all 104 code questions first, MRR 0.964 against 0.951 of Meilisearch; better with word forms and typing
  errors (German stemmer), weaker with compounds. Meilisearch stays the default. The comparison and the variants measured
  are in docs/search.md; the tests start both engines in throwaway containers.
- Proper search, M-12 part one (ADR-24): through a search service, Meilisearch v1.56.2 by default, behind one interface
  for engines (`internal/search`). Error codes such as `E-1042`, `0x80070005`, `ORA-00942`, `HTTP 503` or `KB5034441`
  go into fields of their own and are looked up exactly, never through a typing error, and come first; then the full
  text ranks title before headings before text, exact words before typing errors, newer only on a tie. German compounds
  are split, and title and heading stand a second time unsplit, so typing errors in whole words are still found. Pages
  are indexed in sections (ADR-22): a hit opens at its block or heading, scrolled there, the matching words marked.
  Readers are a filter in the index, every tenant has an index of its own (ADR-23), and passwords (ADR-7) never reach
  it. The index is fed from a queue in the database, filled by triggers on objects, blocks and accounts, so a lost index
  is built again from the queue (`underdoc reindex [--tenant <id>]`). Queue and index are per tenant, with row-level
  security like every table, and one indexer runs for each tenant. Without a search service, or while it does not answer, titles are
  searched and the answer says so. Settings: `UNDERDOC_SEARCH_ENGINE`, `UNDERDOC_SEARCH_URL`,
  `UNDERDOC_SEARCH_KEY_FILE`, `UNDERDOC_SEARCH_INDEX` (docs/search.md).
- Ctrl+K opens a quick search anywhere but in the editor: pages and folders by title at once, hits of the full text as
  you type with place, section and a marked snippet. The results page has filters by type, collection, author, the time
  of the last change and tags, and the strategy *exact words*.
- A German test corpus of 201 pages and 179 questions measures the search (hit@1, hit@5, MRR); every code being the
  first hit is a hard gate. Meilisearch: all codes first, hit@1 93 %, MRR 0.951 (table in docs/search.md). The search
  tests run against real engines in throwaway containers (`scripts/search-engines.sh`, images pinned by digest in
  `scripts/search/`, moved by Dependabot); `scripts/check.sh` fails without Docker instead of skipping them.

- Every state on `main` is published as a container image, `ghcr.io/ollornog/underdoc:main-<commit>`, for amd64 and
  arm64 (`.github/workflows/image.yml`; after a Dependabot auto-merge the merge job starts it, too). The same check as in the CI (`scripts/check-image.sh`) runs first; a manual run
  on another branch builds both platforms and pushes nothing. The build stage of the `Dockerfile` cross-compiles
  (`--platform=$BUILDPLATFORM`, `GOARCH=$TARGETARCH`), so arm64 needs no emulation. The JavaScript stage runs on the
  build platform, too.
- The web editor of M-8, part one (ADR-21): a page of the database is written in one editor, CodeMirror 6 with a live
  preview of UnderDoc's own. Markdown looks rendered away from the cursor and shows on the lines being written; headings,
  emphasis, code, lists, to-dos, quotes, links, rules and tables convert as they are typed, and an embedding shows as a
  box that names what it embeds, with a link. The blocks are joined with hidden marker lines, the same as in the export;
  each block shows its handle there, whose menu copies the address, comments, moves the block and deletes it. A bar above
  the page turns lines into another kind and formats the selection (also Ctrl+B, Ctrl+I, Ctrl+K); `/` at the start of an
  empty line opens the kinds. Pasting an address alone on an empty line embeds it. See docs/editor.md.
- Autosave about a second after typing stops and when the editor is left: changed blocks are written, a paragraph written
  before or after a block becomes a block of its own with its ID and marker, a block whose marker is gone is deleted, one
  used elsewhere only after a question in place, and a changed order is moved. Opening and leaving a page without typing
  writes nothing. A page changed elsewhere meanwhile is never overwritten: it is loaded again with a notice.
- The JavaScript part in the pipeline: `web/editor` is built with esbuild from its lockfile (exact versions, all MIT),
  with the licence texts shipped next to the bundle. The bundles are not committed: `scripts/build-js.sh`, in
  `scripts/check.sh` and the CI, and a stage of the Dockerfile install from the lockfile, run the tests of the editor's
  plan of saving with Node and build them. Dependabot watches `web/editor`, and its updates merge by themselves when the
  checks are green, as Go modules now do too (the auto-merge named the ecosystem `gomod` instead of `go_modules`).
- The editor is loaded when it is needed, not with every page, and the browser keeps assets of a release for a year.
- The look of the editor's bar, the handle on a block's marker line, the question before a used block goes, the box of an
  embedding and the slash menu comes from C22; the editor bundle keeps only what is its own.
- A block is moved by dragging its handle, in reading and in the editor, with the mouse or a finger: the block lifts, a
  gap opens between the blocks where it would land, the drop moves it (reading through the move API, editing in the text,
  which saving moves on the server: same ID, same text, no new version), and Escape gives up. A click still opens the
  menu of the handle, whose Move up and Move down stay the way with the keyboard. Only who may change the page drags.
- In blocks of the database a single newline is a line break, as in Obsidian or Notion: two quote lines, or two lines of
  a paragraph, no longer run together when read. The Markdown does not change; pages of the folder source keep standard
  Markdown.
- The bar of the editor is a row of the window while a page is written, between the head and the page, full width and
  flat, and stays in place while the page scrolls.

### Removed

- The editor trial of M-8 (`/dev/editor-trial`, `web/editor-trial`: Milkdown, TipTap and CodeMirror side by side): ADR-21
  kept it until M-8 closes. Its packages leave the build, the image and the licences with it, among them the KaTeX of
  Milkdown that Dependabot reported. It lives on in the history of the repository, last in commit `d9b5bdb`.
- The block editing of M-28, one Markdown field per block with a bar floating above it and a plus beside it: the editor
  carries all of it (ADR-21). Reading, the handles, their menus and comments stay.

### Fixed

- A name being typed for a new folder or a new collection of files kept neither its text nor the focus when the
  workspace drew anew meanwhile (a toast going, an upload ending): the field showed "New folder" again, and what was typed
  next went nowhere. The field now keeps both, and "Move to the files" of an attachment keeps the focus for the keyboard.
  Found by `TestFilesByHand` in CI, whose toast of a restore went just after the test had started typing.
- `TestShiftAfterAPlainClick` clicked again right after a plain click, while the page it opened was still loading; when
  the page came between the press and the release of the next click, its redraw swallowed that click. The test now
  waits for the page first.
- The browser tests of the files waited only for the uploads and the folders; they now also wait until no request of the
  view is under way (`FS.busy`) and no toast waits to go, since each of them draws the workspace anew and can swallow a
  click whose press and release it falls between.
- A test holds the browser-test helper `rect` to putting the mark of a probe back in the step that measures: it redraws a
  button every two milliseconds and measures it fifty times (TestFilesByHand had failed in the CI when a redraw came
  between finding and measuring, 2026-11-17).
- On a touch screen, a finger on a table in the editor puts the cursor into the cell it touched, so the table tools show;
  before, the caret stayed outside the drawn table.
- Browser tests measured an element found by its text in a second step; when the workspace drew anew in between, the
  mark was gone and the test failed (`TestFilesByHand` under load). Measuring now puts the mark back in the same step.
- The browser test of dividers and moving in nav3 reloaded the page before the drop was written; on a busy machine it
  failed. It waits for the order on the server first.
- The editor knew it had the focus again only from a focus event: after the workspace drew anew around it, a click into
  the text left the boundary of a used block hidden. A press or a selection now tells it.
- The browser tests no longer fail under load: the workspace draws itself in the background, and tests that reached for an
  element at once got `null` ("Uncaught") when the machine was busy. `eval` now tries again, within the same 5 s as
  `waitFor`, while the only problem is an element that is not there yet; `rect`, `clickOn`, `probe` and `rowNamed` wait for
  their element, and tests that reload the page after a change of the settings wait until the settings reached the
  account (`settingsSent`) instead of racing the 600 ms save.
- Go toolchain 1.27.3 → 1.27.4 (`go.mod`, `.github/audit/go.mod`, build image `golang:1.27.4-alpine3.24` by digest):
  `net/http` in 1.27.3 carries GO-2026-6859, so the scheduled `audit` on `main` was red since 2026-11-11.
- Dragging a block works on real pages (2026-11-10): the browser no longer takes the pointer away when text is selected
  over the handle (it started its own drag of the selection and cancelled the pointer), a held block shows in its gap
  with its first lines only, so the places around a long block (a whole section of an imported file) are in reach, and
  the page scrolls by itself while a block is held at the top or bottom edge, so it goes to places out of view.
- The menu of a block's handle is as compact as the right-click menus and looks like them; in the editor it no longer
  shows blank lines between its items, and near the bottom of the window it opens above the handle.
- The editor keeps the rhythm of the page as it is read: a blank line between paragraphs or blocks, away from the
  cursor, is as high as the space after a paragraph (two paragraphs stood 48 px apart, now 34.4 px, as when read), and
  its lines are as high as when read.
- At phone width nothing is cut off and everything can be reached, before the phone has a layout of its own (M-28):
  nav3 had no width when nav2 and nav3 lay over the window, so its lists, the search and the profile were out of reach;
  the handle and the plus beside a block and its speech bubble stood outside the window and showed only to a hovering
  pointer; the menu of a block started off the screen; the windows of commented pages scrolled sideways; and the label
  Administration in nav2 was cut off. A touch screen now shows the handle, the plus and the bubble faintly (C22 block
  `.c22-app-workspace`, vendored from its main, 9f331aa).
- A long press on a touch screen is the right click (ADR-17): it opens the same menu, so the trash, the problems and the
  menus of entries and tabs are in reach on a phone; where the browser opens a menu of its own, only one opens, and the
  lifting finger clicks nothing. Browser tests at 390×844 and 360×740 walk the main states with real touch input and fail
  for anything cut off, covered, hidden behind hover or making the page scroll sideways.

- A link in a block of the database to another object of the database, `/o/<id>`, showed as broken, since only the folder
  source was asked. It now stays a link, and the list of problems checks it against the database.
- A new page made in a collection of the folder source or of the demo was a mock-up that could not be written. New, the
  right click ("New page below it") and New in the head now make pages only in collections of the database and say why
  elsewhere.
- An entry could not be dropped onto an empty collection inside another: while the pointer waited on its middle, the held
  entry slid it away from under the pointer, and the waiting began anew. It now stays where it is while waiting.

### Changed

- Pages of the folder source take no comments: they are files and read only, and comments there were never kept. Their
  speech bubble is greyed and says so; comments are kept on pages of the database. The actions of a comment (reply,
  resolve, edit, delete) have icons.
- Reading and editing: a square button, icon only, at the left of the head of a document, as tall as path and title,
  turns the document between reading (book) and editing (pencil, filled); where a page cannot be edited it shows a lock
  and says why (folder source, demo, only read or comment). The pencil in the head is gone. New in the head of a
  collection in nav3 is gone where one may not add anything there (folder source, only read or comment). A setting under Profile, Documents, opens documents for editing (it follows the account).
- Editing a block as Docmost shows it (the idea only, no code): a bar floats above the block being written, to turn it
  into another kind (text, headings, lists, to-do, quote, code), make the selection bold, italic, struck through, code or a
  link (Ctrl+B, Ctrl+I, Ctrl+K), move the block up or down, duplicate it, copy its address or delete it. A "+" at its left
  adds a block below and opens the list of kinds, which typing "/" in an empty block opens too, filtered while typing:
  text, headings, lists, quote, code, divider, table, embed; image and file are greyed until there is a store for files.
  A block is saved when it is left. New API: `POST /api/pages/{id}/blocks/{block}/move`.
- The browser tests say why Chrome does not answer: whether it still runs, and the end of what it wrote to stderr. Once,
  on the CI, Chrome gave no answer to its first tab, and the test could not say more.
- Moments go out of the API in UTC as RFC 3339 (`…Z`) and show in the time zone of the device: when a page changed and was
  made, its versions, the trash. Before, the server wrote them in its own time, which showed UTC on the test system.
- Private notes stay at the top: they no longer move into another collection, and dragging offers them no place there.
- Dragging entries of nav2 and nav3 now comes from C22 (`app-workspace-drag.js`), as the maintainer approved it; UnderDoc
  only decides what may go where and what a drop does. A test now carries an entry somewhere else before Escape, to prove
  that Escape puts it back.
- The settings of a collection are part of its overview in the window, for the owner and whoever may edit; "Settings and
  name" opens it ready to rename, and nav3 keeps its tree instead of giving way to a settings panel.
- A right click menu stays inside the window: at the edge it moves in, and a long one scrolls. The menu of a tab and New do
  the same; before, a moved place could be overwritten right after it was set.
- The gear of the settings is Lucide's, as the drawn one read as a sun; its licence ships in `assets/app/licenses`.
- While dragging, nothing lights up under the pointer; waiting to put an entry below another takes 0.3 s instead of 0.5 s.
- Each open collection in nav3 begins with a band of its own, set apart from the one before. New keeps its scrollbar.
- ADR-16 settles what sets a folder apart from a collection: a collection can hold collections, which can be shown in nav2,
  a folder shows only in nav3; either turns into the other.
- A dragged entry moves through the list as it looks when held, instead of a picture floating over the rest. Up and down it
  always goes between entries; waiting a moment on the middle of an entry puts it below that entry, moving to the right does
  at once. An entry dropped on a collection of nav2 goes into that collection.
- A plain click is where a range starts: Shift and click right after it takes the range, Ctrl and click adds to it.
- Everything opens in the general tab, and while a single window holds nothing else, neither window nor tabs are drawn; a
  window button beside New holds what the tab offers. A second tab or window brings the tabs back.
- New in a collection puts what is needed most first: a page, then tables, databases, records, boards and lists.
- The name of an open collection in nav3 is larger; the line of a collection kept open in nav2 is slimmer, still thicker
  than one open for now; the filter of New has no box of its own.
- Ctrl and click selects entries of nav2 and nav3, Shift and click a range; dragging one moves all of them in their order, and
  dropped on a window they open there as tabs. Keeping a collection open is now a double click or the right click, and a
  middle click opens an entry as a tab of its own (both were also Ctrl and click before).
- Dragging in nav3: the middle third of an entry puts the dragged one below it, framed, while nothing moves; above and below
  that it lands before or after, and the left edge of the ghost picks the depth: level with the entry above, beside it;
  further right, below it; further left, out towards the top.
- Lifting an entry no longer makes the list jump.
- In nav2 an entry open for now draws a thin line on the left, one kept open a thick one.
- New in a collection lists the types in blocks (write, data, media, organize), each sorted by name.
- Dragging in nav2 and nav3 pulls the entry out of its list, and the others move apart where it would land, live, instead of
  a line on one edge or the other of each entry. Up and down picks the place; in nav3 moving it to the right puts it below
  the entry above it. Where one may only read, no gap opens. The private lists of nav2 sort the same way.
- New in nav2 and in the head of a collection is one menu in groups, and typing filters it; arrows and Enter pick. In nav2 it
  offers a collection, a line and a space; Import and Link come later. A new collection is made at once as
  "New collection (n)" with the first free number, and its name is written in place, in nav2 or nav3.

- nav2 tells kept open from open for now: only a collection kept open draws the line on the left, which replaces the pin.
  The favorite star is gone; nav2 keeps the order it is dragged into, with its lines and spaces. While dragging in nav2, a
  line shows where the entry lands. At the step Text, New in the head of nav2 is no longer cut off, and the first
  collection in nav3 has no line above it.

- The interface is now the approved mockup itself, with its sizes, spacing, clicks and made-up content where nothing real
  exists yet (sprint 2026-11-08). It draws itself in the browser and reads its data from a small JSON API; its look is the C22
  block `.c22-app-workspace`. Real so far: the account, the collections of the database and of the folder source with their
  trees, pages loaded when opened, full-text search on the server, making and renaming collections, and links between pages.
  The made-up collections of the mockup are marked “(demo)”. The browser keeps widths, favorites, pinned collections and the
  theme until the account stores them.
- `/dev/ui` shows the workspace with only the content of the mockup.
- The interface is the workspace of ADR-16, first part (M-28): a top row with name, search and New, nav1 with the views, nav2
  with the collections, nav3 with the open collection, and the window with its head. What is not built yet stays in sight,
  greyed out. A click in nav3 opens the object in the window without reloading the list, and a double click or the arrow shows
  or hides what is in it. The right edges of the columns can be dragged, nav1 and nav2 snap to four steps, and the browser keeps
  the widths and the theme.
- The look follows the approved mockup: compact headings and spacing in pages, tables framed with a shaded head, the way
  above the title starting at the collection, small tags, a larger title, all tools of the window (those not built yet greyed
  out), a status line with words, characters and reading time, the list of tabs at the end of the tab row, and the entries
  of a collection or folder as framed rows with their kind.
- The workspace is now the C22 building block (C22 T-13): the structure, the column steps, the lists, the drawer, the menus
  and the right-click menu of an entry come from C22, and UnderDoc keeps only its own sizes from the mockup. The stored
  column widths start over once.
- The profile is a view like the others: its sections in nav2, the chosen one in nav3, and the administration set apart for
  administrators. A click in nav1 or nav2 changes only the columns, the window keeps what it shows (ADR-16).
- Every object opens at `/o/<id>` (ADR-18). Links to `/d/<id>` move there.
- The title stands in the head of the window with the breadcrumbs, the tags and the description, not in the content.
- The CSP allows requests to the own origin (`connect-src 'self'`), so the window can be swapped in.
- `scripts/vendor-c22.sh` takes the licence text of the Inter font from C22 instead of keeping its own copy current by hand.

### Added

- Tags on pages, folders and collections of the database (migration 008): added in the gear and taken off with the x of a
  tag by whoever may change the object, shown in the head and found by the tag filter of the search, also for objects
  never opened. Lower case, each once, letters, digits, - _ and ., at most 40 characters and 50 per object
  (`PUT /api/objects/{id}/tags`). Copies keep them; import reads them from the front matter or `.underdoc.yaml`, as a list
  or a line with commas, and export writes them.
- Deleting something that is embedded elsewhere names those pages in the question, also for what lies below it, and
  counts the pages the viewer may not see without naming them (`GET /api/objects/{id}/uses`).
- The list of problems (M-3), from the right click on the free part of nav2: embeddings and links in the pages one sees
  that point at a page or block that is gone, never there or not to be seen, links of the Markdown that find nothing, and
  what the folder source ran into, each with a way to its page (`GET /api/problems`).
- Import keeps the links between the pages: relative links to Markdown files and `[[page]]` by ID, path, file name or title
  become links to the pages they became, with their anchors; links and embeddings of objects that got a new ID follow.
- In reading, every block of a page has a handle at its left, shown on hover (as Docmost has one, the idea only). Its menu
  copies the address to embed the block, opens the block for editing, comments on it and says where it is used. It
  replaces the address chip on tables of such pages.
- Comments on pages of the database (ADR-18, ADR-20), on the whole page or on one block through its ID, with replies one
  level deep (migration 007, table `comments`). The drawer of the head opens on the whole page; the speech bubble beside
  a block in reading mode, the comment button in the bar of a block while editing, and `commentOn(docKey, blockId)` open
  it on that block. A thread is resolved and opened again by its author or by whoever may edit the page; each author
  changes and deletes their own comments, and a comment others have replied to stays (resolve it instead). The head and
  each block count the open comments. Read only shows the comments without a field; the owner may always comment. A
  deleted block keeps its comments, listed as "a block that is gone", still to be answered. The text is plain, at most
  10000 characters. The made-up documents keep their made-up comments. New API: `GET` and `POST
  /api/pages/{id}/comments`, `PATCH` and `DELETE /api/comments/{id}`; `/api/doc` carries `comments` and `canComment`, each
  block its `comments`.
- Embedding by ID (ADR-18): a block line `::embed{ref="<page id>#<block id>"}`, or without `#<block id>` for a whole
  page, shows that source live, framed, with a link back. "Copy address" in the bar of a block puts its address on the
  clipboard; pasted alone into an empty block, it becomes the embedding and is saved. The database keeps every reference
  (migration 006, table `refs`), so a page lists where it and its blocks are used, and a block in editing shows "Used in n".
  A block in use asks before it is deleted, also when empty. What cannot be shown says why: a missing page or block, a page
  the reader may not see, a loop, or embedding deeper than five levels. Copies of a page embed what the original did.
- Export and import (ADR-13, ADR-19, [docs/export.md](docs/export.md)): `underdoc export <dir>` writes every collection as
  normal files, collections and folders as directories with a small `.underdoc.yaml` for ID, kind, title, access and the
  order with lines and spaces, pages as Markdown with front matter and an HTML comment `<!-- block bN -->` before each
  block, so the block IDs come back without showing in any viewer; private notes of every account go to
  `private/<login>/`. `underdoc import --owner <login> <dir> <title>` reads such a tree, or any folder of Markdown files,
  into a new collection: IDs are kept where valid and free, nothing that exists is changed, and a page without markers is
  split at its headings. Export and import into an empty database give the same files again. The right click menu of a
  collection offers "Export as files", a zip of what the viewer sees (`GET /api/objects/{id}/export`).
- The trash empties as a whole ("Empty the trash", asking in place; `DELETE /api/trash`), and what lies there longer than
  30 days goes for good by itself, checked at start and every hour.
- What others may do is kept in the database (`POST /api/objects/{id}/access`, part of the tree in `/api/state`): the owner
  sets it in the overview of a collection or the settings of a document, below the top also "as above"; others see it
  greyed out. Before, it lasted only until a reload, and the lock of a collection of the database never showed.
- The settings of the workspace belong to the account (`accounts.settings`, migration 004, `PUT /api/settings`, part of
  `/api/state`): the order of nav2 with its lines, spaces and shown collections, what is kept open, widths, zoom and theme
  follow it from browser to browser. A change goes to the server a moment later; the browser keeps a copy, and without the
  database only that. At most 64 KiB, one JSON object.
- Deleting with a trash: the right click on an entry of nav3 or a collection of nav2 ends with Delete, which asks in place
  ("Delete? Yes") before it acts. What lies in the database goes into the trash with everything below it (migration
  `005_trash.sql`, `DELETE /api/objects/{id}`), leaves every list, and windows showing it let go of it; made-up content is
  deleted in the browser only. Whoever may change an object may delete it, a collection at the top only its owner, and
  private notes are never deleted. Lines and spaces still go for good.
- The trash, from the right click on the free part of nav2: a page in the window listing what one deleted or owns, newest
  first, with where it lay, when and by whom (`GET /api/trash`). Restore puts it back where it lay, at the end, with what
  was deleted with it (`POST /api/trash/{id}/restore`); what lay in something that is in the trash too waits until that is
  restored. Delete for good asks in place as well and takes blocks and versions with it (`DELETE /api/trash/{id}`).
- Folders in the database: New in a collection makes one, named in place (`POST /api/folders`), and the right click turns
  a collection into a folder or back (`POST /api/objects/{id}/kind`). A folder shows only in nav3; a collection at the top
  stays one. Collections of the database now have the icon of a collection, not of a folder.
- A collection inside another can be shown in nav2 as well, by the right click ("Show in nav2"); the entry only points at it,
  opens it in nav3 as a heading of its own, and goes with "Remove from nav2". The browser keeps it with the order of nav2.
  This replaces a trial of the same day that showed the children of a collection in nav2 inside its box.
- Duplicating: the right click copies an entry with everything below it, right after it, and Ctrl held at a drop copies
  instead of moving (`POST /api/objects/{id}/copy`, at most 500 objects at once). The copies belong to whoever copies, pages
  keep their blocks and block IDs, and each copy starts its own history.
- Sorting: the right click on anything holding two or more entries sorts its children A to Z or Z to A; lines and spaces
  stay where they are.
- Every entry of a right click menu carries an icon.
- Lines and spaces set entries apart, in nav2 and in nav3: right click on an entry (Line below it, Space below it), on a
  collection head in nav3 (at the top of it) or on the free part of nav2. In nav2 they are the user's own and the browser
  keeps them with the order; in a collection of the database they are objects, shared with everyone who sees it
  (`POST /api/dividers`, `DELETE /api/objects/<id>` for now only for them).
- Dragging an object of the database in nav3 moves it for good (`POST /api/objects/<id>/move`): only where one may edit,
  nothing into itself or below itself, only a collection at the top, and only its owner takes an object into a private
  collection. If the server says no, the list goes back.
- Backlog M-29: recipes as a document type.
- Pages in the database, edited block by block (ADR-19, M-28): New in a collection, or New page below it, makes a page that
  opens with its first block ready to write. Edit in the window turns every block into a plain Markdown field, saved when it
  loses the focus or on Ctrl+Enter; Escape takes a change back. A block gets its ID when it is made (`b1`, `b2` …, never
  given out twice in a page), and a table shows the ID of its block. Every change is kept as a version with who and when,
  listed in the gear. Deleting a block with text asks first, in place. Others may change a page only where they may edit
  (ADR-20). API: `POST /api/pages`, `POST /api/pages/<id>/blocks`, `PUT` and `DELETE /api/pages/<id>/blocks/<block>`,
  `GET /api/pages/<id>/versions`. The look of the fields comes from C22 (`.bedit`, `.bbar`).
- A JSON API for the workspace: `/api/state` (account and collections with their trees), `/api/doc/<id>`, `/api/search`,
  `POST /api/collections` and `PATCH /api/objects/<id>` (rename, for the owner or where others may edit, ADR-20). Changes are
  taken only from this application's own pages.
- PostgreSQL, optional through `UNDERDOC_DATABASE_URL` (ADR-19): accounts from the gate, each with a private collection My
  notes, and collections made with New in nav2 or inside another one from its heading in nav3, with IDs of ADR-18. A
  collection opens its page with owner, what others may do and who sees it. Changes are taken only from this
  application's own pages. The tests start a throwaway PostgreSQL and fail without one.
- The user comes from the headers of the sign-in gate, and without them nothing but the health endpoint answers. A profile
  shows the account, signing out, the theme and, to administrators, the administration still to come.
- `description` and `tags` in the front matter. A click on a tag lists everything that carries it.
- Project skeleton: concept, backlog with milestones and decisions, CI, repository hygiene.
- Plan for versions and audit (ADR-11).
- Logo and favicon.
- Plans for search (M-12) and AI (M-13).
- Plans for the workspace with tabs and windows, favorites and inbox, import and export, calendar, forms, reviews through Git
  and a site builder (M-14 to M-21), and an order of work: one topic at a time, built as modules (ADR-14).
- Plan for a workspace of typed documents with three views: workspace, files and resources (ADR-15).
- How the workspace looks and works: icon column, favorite collections, tabs, windows that come and go, a gear per document, tags and
  search strategies (ADR-16).
- Plans for more document types (to-do list, kanban board, video, PowerPoint, quick notes), settings with modules an administrator
  switches on and off, mail (M-22), IP addresses and devices (M-23), and a private area (M-24).
- Plans for bookmarks (M-25), news from feeds (M-26) and more file sources (M-27). Every window gets its own general tab with a
  history, tabs behave as in code editors, New lands in a chosen collection or in Quick notes, one rule for clicks holds in every
  view, settings move into the profile, and an AI drawer sits at the right edge. The tabs share the top row, the heading moves out
  of the content, and saved searches become a document type (ADR-16).
- Guiding principles for every feature (ADR-17) and a proposal for object IDs and addresses (ADR-18). Every collection sits in nav2,
  a collection is an object that can sit inside another and can be kept open, Ctrl with a click adds without replacing, and the document head keeps the
  title below the breadcrumbs (ADR-16). A draft for the first build with sign-in, account, collections and documents as objects
  (M-28), and mockups for the phone layout (T-10).
- Pages live in the database as blocks with IDs, and Markdown is the way in and out (ADR-19). Who sees and changes what before
  roles arrive: an owner per object, and edit, comment or read for all others (ADR-20). The scope of the first build is settled
  (M-28): PostgreSQL now, comments, tables embedded by their ID, New collection in nav2 and from a right click on free space, and
  private lists that open like collections (ADR-16).
- A click in a list opens the object, also a collection with its page, and a double click or the arrow opens its children. New
  collection sits at the top of nav2, the plus moves to the right of the search field, bookmarks become a document type instead of
  a view, and comments belong to an element of a page (ADR-16, ADR-17, ADR-20, M-25).

### Removed

- The pages rendered on the server, among them `/search` and `/problems`, and their scripts: one interface instead of two
  (ADR-17). The number of problems is in `/api/state`; the list of them comes back in the workspace.

### Security

- A collection could be made inside another one where others may only comment or read. Now that needs the right to edit
  there, as everything else does (ADR-20).
- The Markdown of a block stays text in its field, and its view drops raw HTML like every page; a browser test proves both.
- Names, descriptions, tags and paths from files and from people are shown as text everywhere in the workspace; a browser
  test proves it with names full of markup.

### Added

- A folder of Markdown files appears as a wiki: tree, pages, `[[wiki links]]`, backlinks, table of contents, images next to a
  page, full-text search, recent changes and a list of everything broken (M-1).
- Pages with an `id:` in their front matter keep their address and their links when the file moves.
- Changes on disk appear within seconds, without a restart.
- One binary or one container, read-only access to the folder, a strict CSP and a health endpoint.

### Fixed

- A link to a folder opens that folder instead of counting as broken (B-1).

### Changed

- Documents have their own identity and roles, separate from the folder tree (ADR-13, replaces ADR-3).
- The first milestone is now a Markdown wiki only. Excel, Samba and versions moved to M-2 (ADR-12).
