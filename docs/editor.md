# The web editor

Pages of the database are written in one editor per page: CodeMirror 6 with a live preview of UnderDoc's own (ADR-21).
The text stays Markdown. Away from the cursor it looks rendered; on the lines the selection touches, the Markdown shows.

## How a page becomes one text

A page is a list of blocks (ADR-19). The editor joins them into one text, each block after a marker line, the same marker
as in the export ([export.md](export.md)), with a blank line between blocks:

```markdown
<!-- block b1 -->
## Switches

<!-- block b3 -->
Two of them, in the tower.
```

The editor hides the markers and shows the handle of the block in their place; the cursor never enters a marker line.
Away from the cursor a blank line between paragraphs, and so between blocks, is only as high as the space after a
paragraph when the page is read, and the lines are as high as when read, so the editor keeps the rhythm of the page, as
Typora and Obsidian do (2026-11-10: two paragraphs stood 48 px apart, now 34.4 px, as when read). On the cursor the blank
line is a whole line again; blank lines in code stay lines.
Only markers the editor keeps track of count: those it wrote when the page opened, those saving wrote, and a marker line
that is typed, pasted or brought back by undo as a whole line. A line like a marker that stands in a block, in fenced
code for example, stays text.

## Saving

About a second after typing stops, when the editor loses the focus and when it is left (back to reading, another page,
the browser tab hidden), the editor compares its text with the stored blocks and calls the block API:

| In the text | Call |
|---|---|
| The text of a block changed | `PUT /api/pages/{id}/blocks/{block}`; a part in use whose text is gone asks for its way first ([parts.md](parts.md)), and without one the block keeps its text |
| Paragraphs written before or after the text of a block, a blank line apart, or before the first marker | `POST /api/pages/{id}/blocks` for each, after the block before it; then its marker goes into the text, without moving the cursor or the view |
| A marker is gone (Backspace at the start of a block joins it to the one before, or the text was cut) | `DELETE /api/pages/{id}/blocks/{block}`; a block used elsewhere asks first, in place, then for the way of each use ([parts.md](parts.md)), and comes back when kept |
| The order changed (a block dragged by its handle, Move up and Move down in the menu of the handle, or a cut and paste) | `POST /api/pages/{id}/blocks/{block}/move` |
| Nothing changed | nothing: opening a page and leaving it writes nothing and makes no version |

A block keeps its ID while its text changes, so embeddings, comments and versions stay with it. Text that a joined block
brought along stays in the block it was joined to. A copied marker makes a new block; the original keeps its ID.

Before it writes, the editor reads the page again. If someone changed it meanwhile, nothing is overwritten: the page is
loaded again with a notice, which offers to copy what was written since the last save (M-10 brings editing together).

## Writing

- Markdown typed converts as it is typed: headings, emphasis, code, lists, to-dos (`[ ] ` becomes `- [ ] `), quotes,
  links, rules, tables. `[[links]]` and directives such as `::embed{…}` stay as written; an embedding away from the
  cursor shows as a box that names what it embeds, with a link.
- The bar is a row of the window while its page is written, between the head of the window (reading or editing, path,
  title) and the page, over the full width and outside what scrolls, so it stays in place (2026-11-10): Turn into (text,
  headings, lists, to-do, quote, code) for the lines of the selection, and bold, italic, strikethrough, code and link for
  the selection, and whether the page is saved. Ctrl+B, Ctrl+I and Ctrl+K do the same. With text marked, Copy, Embed and
  Name it as a value ([parts.md](parts.md)). Its buttons keep the selection and
  the focus in the editor. Every button writes plain Markdown, so the round trip stays whole.
- **A block used elsewhere** (embedded in other pages, ADR-22) shows its boundary while the cursor is in it: a light
  outline round its lines and, at the end of its first line, "Used in N places", with the places in its title. So whoever
  writes there knows that other pages change too. Away from the block, or in a block used nowhere, nothing shows.
- **Table tools** (`web/editor/src/tables.js`): while the cursor stands in a table, the bar shows Row above, Row below,
  Column left, Column right, Delete row, Delete column and the alignment of the column (left, center, right, written in
  the row of dashes). Each writes the whole table again as plain Markdown, `| a | b |` with one row per line, and the
  cursor goes to the cell the tool names; a table no tool touched stays as it was typed. The header row and the last
  column stay: a table needs them, and the workspace says so. The tools are a group of the bar that wraps on a narrow
  screen instead of scrolling out of sight, and on a touch screen a finger on the drawn table puts the cursor into the cell
  it touched.
- `/` at the start of an empty line opens the kinds; typing filters them, arrows choose, Enter takes one, Escape closes.
- **Passwords** (ADR-7, [secrets.md](secrets.md)): "Password" in the slash menu, or the lock in the bar at the cursor,
  asks for the value in a dialog with a field that hides it, keeps it sealed on the server and writes only
  `::secret{id="…"}`. Away from the cursor the reference shows as a mask with Reveal, Copy and Change; at the cursor it is
  text. A revealed value is masked again after half a minute. A value typed as text after a label like "Password:" is asked about before the page is
  saved, and can become a password in place ([secrets.md](secrets.md), `src/plain.js`).
- Pasting the address of a block or page alone on an empty line embeds it (ADR-18).
- Pasting or dropping an image uploads it as an attachment of the page and puts `![name](address)` on a line of its own
  where it landed ([files.md](files.md#attachments-of-pages)).
- The handle of a block: Copy address, Move up, Move down, Delete, Comment, the alignment of the block (left, center,
  right; ADR-16), and where the block is used. The alignment is kept with the block, not in its Markdown, through
  `PUT /api/pages/{id}/blocks/{block}/align`; it makes no version, and the editor shows it on the block's lines. Its menu is as
  compact as the right-click menus and looks like them; near the bottom of the window it opens above the handle. Dragged,
  the handle moves the block (below).
- A single newline is a line break, as in Obsidian or Notion: two lines of a quote, of a paragraph or of a list item read
  as two lines, in the editor as when the page is read (below).

## Templates

A new page can start from a template (M-8), after Confluence and Docmost: New at a collection lists them right after
Page, and New in the top bar, with a collection chosen, under "From a template"; there the title typed in the field names
the page, otherwise the template does. Each template shows what it is for under its name, in New at a collection as the
second line of its item and in New in the top bar as a card (C22's `.tplcards`). The page starts with the template's blocks, numbered from b1, each with a version
by whoever made the page, and opens in the editor.

- **UnderDoc's own:** Meeting notes, How-to, Runbook and Decision (`internal/db/templates.go`).
- **Your own:** any page of the database is offered as a template from its gear (Template: off, offered), by whoever may
  change it; it stays where it was written, and whoever sees it can start from it, so a private page offers itself only to
  its owner. A page made from it is a page of its own, not a template, and keeps no tie to it.
- API: `GET /api/templates`, `PUT /api/objects/{id}/template {"template": true}`, `POST /api/pages {"template": id}`. The
  export writes `template: true` into the front matter, and an import reads it back.

## Quick notes

Quick notes are a document type of their own (ADR-16, M-8): a board of small cards, newest first, with a field on top to
add one. Each account has its board in its private notes, made the first time a note is taken; each card is a block of
the board (kind `notes`), so it keeps its ID, versions and comments, and editing the board writes its cards as a page.

- **New** lands in Quick notes or in a chosen collection, and the last choice stays with the account. With Quick notes,
  typing and Enter puts a card on the board without choosing a place first (`POST /api/notes {"md": …}`); with a
  collection, it makes a page there.
- The field on top of the board adds the next card (`POST /api/pages/{board}/blocks`).
- The export writes the board as a page file with `kind: notes` in its front matter, and an import makes it a board again.

## Line breaks

In CommonMark a single newline inside a paragraph is a soft break and shows as a space, so two quote lines `> a` and
`> b` run together. In blocks of the database it is a line break: the server renders them with hard wraps
(`Library.RenderMarkdown`, `markdown.Page.BreakLines`). The Markdown itself does not change, so the round trip stays
identical, and the export writes it as it is. Pages of the folder source keep standard Markdown, since their files are
wrapped by hand. Code keeps its lines; inline code shows a newline as a space, as before.

## Moving a block by dragging

Who may change the page drags a block by its handle, in reading and in the editor (`internal/web/assets/app/blockdrag.js`,
2026-11-10). Pressing the handle and moving it lifts the block: it is hidden, and a gap with the block in it (C22's
`.dropgap`) opens where it would land, always between blocks, never inside one. Letting go moves it; Escape gives up.
A click without moving still opens the menu of the handle, and Move up and Move down in that menu stay the way with the
keyboard. Pointer events, so a finger drags the handle as a mouse does (the handle takes the touch instead of the page
scrolling, and is faintly visible on touch screens). The handles of someone who may only read or comment do not drag.

Real pages have long blocks: a whole section of an imported file is one. So the gap shows only the first lines of the
held block, and the places around it stay in reach; held near the top or bottom edge of the page, the page scrolls by
itself, and the block goes to places out of view. It scrolls by the time held, not by the frames drawn, so a busy
browser gets as far in the same time. The places are measured anew as the page scrolls, as if the held block
were not there. The press on a handle is the module's: it starts no selection of text and no drag of the browser. Before
(2026-11-10), with text selected over the handle, the browser started its own drag of the selection and cancelled the
pointer, and a long block could not be moved down past blocks out of view.

- **Reading:** the drop calls `POST /api/pages/{id}/blocks/{block}/move {after}`; the view shows the new order at once and
  goes back with a notice if the server refuses.
- **Editing:** the editor hides the block's lines and draws the gap as a widget of its own; the drop moves the block's text
  with its marker in the text, and saving moves it on the server. Same ID, same text, no new version; the cursor stays
  where it was in its block, and the view does not jump.

C22's module for dragging the lists (`app-workspace-drag.js`) does not fit blocks: it leaves touch to the long press, and
it moves elements of the list itself, while the editor draws its own lines, so the gap there has to be the editor's.

## The CSP stays strict

CodeMirror writes its base styles with style-mod, which puts them into a `<style>` element in a document. The CSP of
UnderDoc blocks that (`style-src 'self'`, no inline style, no nonce). The bundle gives style-mod a stand-in for the
document that has the document's adopted style sheets but no `<head>`, so style-mod uses a constructed style sheet, as
it does in a shadow root, and the CSP allows that (`web/editor/src/style-mod-adopted.js`). The editor lives in the page
itself, so the styles of the workspace (C22) reach the bar, the handles and the slash menu. Pictures that are not of
UnderDoc itself show as their alt text, never as a request elsewhere.

## Built from C22

The editor and its parts are C22's building blocks, their look the layout pack `underdoc` (T-12 step 3, 2026-11-22); the
editor's own stylesheet (`web/editor/src/editor.css`) holds only the inside of CodeMirror and the live preview.

| Part | C22 |
|---|---|
| The page in the editor | `.editor` on CodeMirror's root |
| The bar | `.workspace-main > .editor-bar`: a `select`, groups `div[role=group]`, the status as `small` |
| A block while reading | `article.typeset > div[data-block]`, its handle `.dropdown-menu.editor-handle` (menu drawn open by the script); in the editor the marker line `.cm-ud-marker[data-block]` holds the handle, `data-active` on the block with the cursor |
| The slash menu | `.command` with `[role=menuitem]`, the chosen one `data-selected`, a later one `aria-disabled` |
| A question in place | `article.item[data-variant=muted][data-size=sm]`, the answers as buttons in its `aside` |
| A block used elsewhere | its lines `[data-used]`, "Used in N places" as `small` |
| A part | `[data-part][data-framed]`, its name in `data-part` (while reading, boxes placed by `parts.js`) |
| A value that looks like a password | `underline decoration-wavy decoration-destructive` |
| An embedding | in the editor `a.badge[data-variant=secondary]`; while reading `figure.editor-embed`, `data-state="missing"` with the reason, or who removed it with Restore and Keep a copy here |
| Comment bubbles | `.comment-anchor > [data-comment]`, the count `data-count` |
| Quick notes | `.input` on top, a grid of `article.card[data-size=sm][data-block]`, the handle at the end |
| What happens where something in use goes | a `.card` over the workspace with one section per thing |
| The source view | `pre.code-block[data-line-numbers]` |

What the editor first lacked in C22 (the space between the elements of a block with several and of an embedding, a
to-do of a list, the handle of the block with the cursor, one frame round the lines of a block used elsewhere, the gap of
a dragged block, the frames of a part over several lines, a handle that takes a finger, handles and bubbles on touch
screens) C22 has had since `ec39f36`; UnderDoc keeps no look of its own for them.

### The workspace around it

The menus, drawers, panels and other views of the workspace are C22's building blocks too (T-12 step 4, 2026-11-22),
after the recipes of C22's `workspace-views.html` and `workspace-more.html`. Every menu and popup hangs inside the
`.workspace`, so the pack gives it the mockup's size; the script draws open what is open. `c22.js` is loaded for the
building blocks outside the workspace that need it (the steps of a dialog); `data-behaviour="app"` on the `.workspace`
keeps it out of the workspace, which UnderDoc drives itself (grips, lists, clicks, menus).

| Part | C22 |
|---|---|
| A right click, the menu of a tab, Opened here before, All tabs, Window and tabs | `.context-menu > [data-cm-panel]` fixed at the pointer or its button (`data-style`), a heading `[role=heading]`, entries `[role=menuitem]`, Delete `data-variant="destructive"` |
| More in the tools of a window | `.dropdown-menu` with its `[data-popover]` |
| New in nav2 and in a collection | `.command` fixed below its head (`data-menu="new"`), groups with headings, a template `div > span + small` |
| New in the top row | `.popover` of the New button (`data-menu="quick"`): where it goes as a `.button-group`, an `.input`, templates and types as `a.item` in a grid |
| The quick search (Ctrl+K) | `.command` (`.udpal`), hits as entries with their place and excerpt |
| A short message | `.toaster > .toast` |
| The gear, the comments, the assistant | `.workspace-main > aside[data-drawer]`: settings as `.button-group`, facts as `dl`, comments as `ol.timeline` (a reply nested, a resolved thread `data-state="done"`), the assistant's `[role=log]` of `.bubble`s, fields in an `.input-group` |
| Profile, settings, administration, a source, the tab Rights | a `.card` in nav3 with `h3`, rows of a name and a control, `.item` lists, small outline buttons |
| Mail and news | `a.item` with sender and `time` over the subject, `data-state="unread"`, the open one `aria-current`; the message `dl` first in the article |
| To-dos | `label.item` with a checkbox, done is checked (C22 strikes it through), the due date a `time`; in nav3 `data-size="xs"` |
| The month, a board, a saved search | `table.month-sheet`, `.board` of `.card` columns, `.filter-bar` with hits as `.item`s, each straight in the window's `section` |
| Content without blocks | `.alert` for a hint, `.table-container > table.table`, `code` for a value, `a.item` for what lies below, `.empty[data-dropzone]`, `figure.card` for a preview |

What the step first lacked in C22 (the size of menu entries in a context menu, the icon in the head of a drawer, the
current hit of Find as `mark[aria-current]`, the element of open comments as `.comment-anchor[data-comment-focus]`,
the drawer below the head of the window, modules as switches) C22 has had since `bad252a`; UnderDoc keeps no bridge for
them.

### Dialogs and pages outside the workspace

The dialogs and the pages without the workspace are C22's building blocks as well (T-12 step 5, 2026-11-22), after C22's
`rights.html`, `steps-dialog.html`, `admin-panel.html` and `public-page.html`. The root class of C22's old app workspace
is gone (step 6): UnderDoc emits none of its classes.

| Part | C22 |
|---|---|
| The rights dialog | `dialog.dialog`: owner and inheritance as `.item`s with their action, the roles here as a `.table` with a role `select` and a ghost button with a cross to take one back, each name with the icon of its kind (globe, users, user), a role from above as `.badge-split` (origin, role), share links as a `.table` |
| A new collection | `form[data-steps]` with `ol.steps`: the name, then Share with; Enter in the name makes it at once, its maker's alone |
| The passwords of a page | `dialog.dialog` with an `.item` per password, versions and readers below it |
| The log in the tab Rights | `ol.timeline[data-size=sm]`: what as a badge (a wrong password destructive), who and when |
| Keeping the log | `.input-group` with the unit after the field |
| The table view of a spreadsheet | `.tabs` for the sheets, the sheet `table.table[data-variant=grid][data-sticky=sheet][role=grid]` in a `.table-container`, numbers `data-type="number"`, a range `aria-selected` on its cells and on the letters and numbers of its columns and rows, the cell it began at `data-active`; names and tables as `.item`s |
| The page behind a share link | one column `main.grid`, the head, `.alert`s, the frozen page in `.typeset`, files in a `.table-container > table.table[data-wrap]` |

Choosing whom to share with stays a `select` with groups: C22's grouped `.combobox` needs Basecoat's script, which the
workspace does not load (Basecoat would take over the menus the workspace draws itself).

## Build

The editor is built from `web/editor` with esbuild into `internal/web/assets/app/editor/`: `editor.js`, `editor.css` and
`LICENSES.txt`, the licence texts of every bundled package (all MIT). The built files are in the repository, so building
UnderDoc needs only Go.

```bash
cd web/editor
npm ci --ignore-scripts
npm test             # the plan of saving, without a browser
npm run build        # into internal/web/assets/app/editor/
```

The versions are exact, stable and were at least two weeks old when chosen (`npm install --save-exact
--before=<date>`). The bundles are not in the repository: `scripts/build-js.sh` (in `scripts/check.sh` and the CI)
installs from the lockfile in a temporary copy, runs the tests and builds into the assets the binary embeds, and the
Dockerfile does the same in a stage of its own. So an update of the lockfile, for example from Dependabot, is built and
tested by the checks as it is, and merges by itself when they are green. After pulling a changed lockfile, run
`scripts/build-js.sh` once before `go build`. The editor trial of M-8 (`web/editor-trial`, `/dev/editor-trial`) went
when M-8 closed (ADR-21); it lives on in the history of the repository, last in commit `d9b5bdb`.

**Loading:** the workspace does not load the editor with every page. It comes when it is needed — on the way into
editing, when the pointer reaches the reading/editing button, or in an idle moment when editing is likely (the setting
opens pages for editing, or a page one may change is open). Its URL carries the version of the release, and the server
lets the browser keep such assets for a year, so it is fetched once per release.
