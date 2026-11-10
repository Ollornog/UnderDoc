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
Only markers the editor keeps track of count: those it wrote when the page opened, those saving wrote, and a marker line
that is typed, pasted or brought back by undo as a whole line. A line like a marker that stands in a block, in fenced
code for example, stays text.

## Saving

About a second after typing stops, when the editor loses the focus and when it is left (back to reading, another page,
the browser tab hidden), the editor compares its text with the stored blocks and calls the block API:

| In the text | Call |
|---|---|
| The text of a block changed | `PUT /api/pages/{id}/blocks/{block}` |
| Paragraphs written before or after the text of a block, a blank line apart, or before the first marker | `POST /api/pages/{id}/blocks` for each, after the block before it; then its marker goes into the text, without moving the cursor or the view |
| A marker is gone (Backspace at the start of a block joins it to the one before, or the text was cut) | `DELETE /api/pages/{id}/blocks/{block}`; a block used elsewhere asks first, in place, and comes back when kept |
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
  the selection, and whether the page is saved. Ctrl+B, Ctrl+I and Ctrl+K do the same. Its buttons keep the selection and
  the focus in the editor. Every button writes plain Markdown, so the round trip stays whole.
- `/` at the start of an empty line opens the kinds; typing filters them, arrows choose, Enter takes one, Escape closes.
- Pasting the address of a block or page alone on an empty line embeds it (ADR-18).
- The handle of a block: Copy address, Move up, Move down, Delete, Comment, and where the block is used. Dragged, it moves
  the block (below).
- A single newline is a line break, as in Obsidian or Notion: two lines of a quote, of a paragraph or of a list item read
  as two lines, in the editor as when the page is read (below).

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
`scripts/build-js.sh` once before `go build`. The editor trial of M-8 (`web/editor-trial`, `/dev/editor-trial` with the
dev switch or for administrators) is built the same way until M-8 closes.

**Loading:** the workspace does not load the editor with every page. It comes when it is needed — on the way into
editing, when the pointer reaches the reading/editing button, or in an idle moment when editing is likely (the setting
opens pages for editing, or a page one may change is open). Its URL carries the version of the release, and the server
lets the browser keep such assets for a year, so it is fetched once per release.
