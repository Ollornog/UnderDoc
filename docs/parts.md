# Parts, sections and embedding

A page is a list of blocks (ADR-19), and anything of it can be embedded elsewhere by its address (ADR-18). ADR-22 adds
what can be reused below a page and how it lives on. Three units can be reused:

| Unit | Address | What it is |
|---|---|---|
| a block | `<page>#b3` | a paragraph, table, list or code block |
| a section | `<page>#b3.section` | the block `b3`, which opens with a heading, and the blocks after it up to the next block that opens with a heading of the same or a higher level; it grows with the original |
| a part | `<page>#b3.t2` | a few words inside a paragraph, a heading, a list item or a table cell of `b3` |

Every part has exactly one home, its block, and so its page: owner and rights are the page's, who wrote it comes from the
versions of the block. Nobody types IDs: marking text gives the part its ID (below).

## A part in the Markdown

A part stays where it is. Two point markers in the Markdown of its block give it an ID local to the block:

```markdown
Error 42: <!-- part t1 -->the disk is full<!-- /part t1 -->. Free some space.
VAT is <!-- part t3 name="vat" -->19 %<!-- /part t3 -->.
```

- The markers are HTML comments, so every Markdown viewer leaves them out, and the export keeps them as they are
  ([export.md](export.md)): an import gives every part its ID back.
- A part with a `name` is a **value** of M-3: the same markers, boundaries and choices, with a name to find it by.
- An opening marker pairs with the first closing marker of the same ID after it **in the same paragraph** (or heading,
  list item, table cell). A marker without its partner, a second opening marker of an ID already taken and a marker in
  code are no part. A paragraph that starts with a marker stays a paragraph, and a heading keeps its anchor.
- Parts may overlap or nest. A part ends when its text is gone. Its ID is never given out again while something still
  embeds it.

## Cutting through formatting

A part may start or end inside bold, italic, strikethrough or the text of a link. Embedded or copied, it is cut out of
the parsed paragraph (goldmark), and formatting open at its edges is closed and opened again:

```markdown
**very <!-- part t2 -->important to all** customers<!-- /part t2 -->      →   **important to all** customers
see [the <!-- part t1 -->manual](/o/7k2m9q4x1c) now<!-- /part t1 -->     →   [manual](/o/7k2m9q4x1c) now
```

The page shows the part exactly as it reads in its paragraph: the HTML of the paragraph is cut between the two markers
and the inline elements open at the edges are closed and opened (`markdown.CutPart`). The Markdown of a part on its own,
for a copy, is written the same way and checked against that HTML; where the formatting cannot be written so (two runs of
the same delimiter that would merge at the edge, such as `*!*0*0*`), it comes as its plain text. A line of the part that
would read as a heading, list or quote on a line of its own is escaped. A table test with many cases and a fuzz test
(`internal/markdown/parts_test.go`) hold this.

## Marking

Marking a stretch of a block (`markdown.MarkPart`) snaps its edges outwards to the edges of **atomic elements**, which a
part never cuts: inline code, an image, an autolink, a wiki link, raw HTML, an inline embedding and the address of a link
(`[text](address)`: the text may be cut, the address not). A part never leaves its paragraph: a selection over two
paragraphs, list items or table cells is refused; for more, embed the block or its section. The markers go where the
paragraph reads exactly as before, inside or outside a run of `**`, `_`, `~~` or `[` next to the edge, whichever keeps it.

| Call | What it does |
|---|---|
| `POST /api/pages/{id}/blocks/{block}/parts {"start", "end", "name"}` | marks the stretch as a part (with a name: a value) and keeps the change as a version; answers the new Markdown, the part, its address `ref`, its `url`, and where its text now lies (`start`, `end`). Only who may change the page. |
| `POST /api/pages/{id}/blocks/{block}/excerpt {"start", "end"}` | the Markdown of the stretch as a **copy**: plain text with formatting closed, without markers, and the address it came `from`. Whoever sees the page. |
| `GET /api/objects/{id}/uses?ref=b3.t2` | where an address is used, place by place: page, title, block, and whether the viewer may change that page; how many more places the viewer may not see. For a block, the uses of its parts and its section count too. |

Offsets are UTF-16 code units of the block's Markdown, as the browser counts them.

## Embedding

- A line `::embed{ref="<page>#b3"}`, `…#b3.section` or `…#b3.t2` on its own embeds a block, a section or a part as a box
  with a link back. A part shows as a paragraph there.
- Inside a paragraph, `:embed{ref="<page>#b3.t2"}` embeds a part in the flow of the text, as a value of M-3 shows. It
  reads as the text around it; where it comes from shows on hover. In code it stays text.
- An embedding is rendered for each viewer each time: live, never wider than the viewer may see (concept: embedding
  bypasses nothing). What cannot be shown says so in place: not to be found, a block or part no longer there, a block that
  opens no section, a loop, nesting deeper than five.
- Every embedding is kept in `refs` with its target (`b3`, `b3.t2`, `b3.section`), so each block shows where it, its
  parts and its section are used (`parts`, `sectionUsedIn` of a block in `/api/doc`), and the list of problems names every
  embedding that does not resolve.
- The markers of a part show in the HTML of its own page as empty `<span data-part-open="t2">` and
  `<span data-part-close="t2">`, so the page can show the boundary of a part; they never travel into an embedding.

## When something in use goes

Deleting a block that something embeds, deleting the text of a part in use (the part ends when its text is gone), or
deleting a page of which something is embedded elsewhere always asks first. The workspace asks before it saves (`POST
/api/pages/{id}/blocks/{block}/lost {"md": …}` or `{"delete": true}` says, without changing anything, what would go and
where it is used), and the server refuses such a change without a way with 409 and the same list. A dialog names each
thing with every place of use and offers three ways, with nothing chosen in advance and nothing deleted before each thing
has its way; with more than one, "The same for all" takes the first choice; "Keep it" gives up, and the block or its text
comes back:

| Way | What happens |
|---|---|
| **Delete everywhere** | It goes (`removed`), and every embedding shows "Removed by … on … from …" in its place, never a quiet gap. **Restore** on the placeholder (for who may change its page) puts it back, blocks with their IDs where they were, a part as a paragraph at the end of its block, and every use shows it again. A page in the trash shows the same placeholder until it is restored. |
| **A copy in every use** | Each embedding in a page the viewer may change becomes plain text in its place, without markers; the version of that block says `copy of <address>`. Every other page keeps a placeholder, its owner gets a notice, and **Keep a copy here** on it writes the copy there. |
| **New master** | The viewer picks one use in a page they may change. The content moves there in place of its embedding (a part with a new part ID in that block, a block or section as blocks of that page; the rest of a section leaves the old page with it), every other reference in `refs` is rewritten to the new address in the same transaction, and the old address answers with a redirect (`redirects`, `GET /api/redirect?ref=`), also for embeddings written later with it. The owner of that page gets a notice. |

A block that goes takes its parts and its section with it, one way for all. A whole page has no place inside another, so
its own embeddings get everywhere or copies. Of a page the viewer may not see nothing is told, not even that it went.

- API: `PUT …/blocks/{block} {"md", "ways"}`, `DELETE …/blocks/{block} {"ways"}`, `DELETE /api/objects/{id} {"ways"}`,
  with `ways` keyed by the target (`b3.t2`), the address, or `*` for all: `{"way": "everywhere" | "copies" | "master",
  "master": "<page>#<block>"}`. `POST /api/removed/{id}/restore`, `POST /api/removed/{id}/copy {"page"}`.
- Notices (`notices`, `GET /api/notices`) are kept for the inbox of M-16, which shows them; until then the dialog says
  that owners are told.
- The version history notes where a copy came from and where moved text went (`origin` of a version), also for a text
  copied through the clipboard ([below](#where-a-copy-came-from)).
- **Moving a block into another page** (`POST …/blocks/{block}/move {"page", "after"}`, who may change both pages) takes
  its parts and its section along under a new block ID there; every reference is rewritten in the same transaction and
  the old addresses redirect. The menu of a block offers it while reading and while editing (**Move to page**,
  then a page of the database): it goes to the end of that page. While editing, the page is saved first and the editor
  opens again on what is left.
- **A page in the trash** keeps nothing that moved to a new master when it went: a block goes, a part leaves its block
  with its markers, and the version of the block says where it went. Restoring the page does not bring it a second time.

## In the editor and while reading

- **Marking:** with text marked inside one paragraph, the bar offers **Copy** (plain text with no tie, through the
  excerpt above; over several blocks the Markdown as it stands, without markers), **Embed** (the text becomes a part, or
  stays the one it is, and its address goes to the clipboard) and a field **Name it as a value** with its button (the same,
  with a name). The editor saves first, then the server snaps the edges and writes the markers, and its answer becomes
  the block's text.
- **Pasting** the address of a part into a paragraph embeds it there (`:embed{ref=…}`); alone on a line, any address
  embeds as a block, a section (`#b3.section`) or a part.
- **Markers** of parts are hidden in the editor and atomic: the cursor jumps over them, and a marker cannot go on its own
  while text of its part is left (it is put back). When all text of a part is gone, both markers go, and saving asks what
  happens where the part is used.
- **Boundaries only when needed:** with the cursor in a part, the editor draws a light outline round its text and a
  label, "Used in N places" or "Not used elsewhere", with "Value <name>" in front for a value. While reading, the pointer
  over the text of a part, or a finger on it on a touch screen, shows the same outline and label (`parts.js`, over the
  boxes of the range between the two markers, so parts that cut through formatting or overlap show whole).
- **Values:** `GET /api/values` lists every part with a name in the pages the viewer sees, with its text, its address
  and how many places use it (`uses`, counted in pages the viewer sees).
- **Snippets:** `GET /api/snippets` lists the parts without a name, sections and blocks that pages the viewer sees embed,
  with a short text and the same count. Pages the viewer may not see are neither counted nor named.
- **The Resources tab** (ADR-15) shows both with a database: Values and Snippets list them, and each opens with its page.
  Databases and external sources stay the mock-up of the first build until M-4 and say "(demo)".
- **The facts panel** of a page names what it embeds, in the order of its blocks: a value by its name and the page it
  lives in, a part, a section, a block or a whole page by its kind (`embeds` of `GET /api/doc/{id}`).

## Tables of spreadsheets

A sheet, a range, a defined name or an Excel table of a spreadsheet file is embedded the same way, by an address local
to its file, `<file>#x3`, and a name of one cell inside a paragraph as a value: [tables.md](tables.md#tables-in-pages).

## Diagrams

A fenced block with the language `mermaid` is a diagram (T-3). The server marks it (`figure[data-mermaid]`) and keeps its
text; a value or another part embedded in it with `:embed{ref="…"}` becomes its text, as in a paragraph, so one change
of a value shows in every diagram that uses it. The browser draws it with Mermaid 12, bundled from the lockfile in
`web/mermaid` and served by UnderDoc itself.

- **Under the strict policy.** Mermaid writes inline styles, which the policy of the workspace forbids (no
  `unsafe-inline`), and it reads text anyone may have written. So it draws in a frame of its own,
  `/assets/app/mermaid/frame.html`: sandboxed without an origin of its own (`sandbox allow-scripts`, in the attribute and
  in its own policy), its script only from UnderDoc, inline styles allowed, no inline script, no connection, and only
  UnderDoc may hold it (`frame-ancestors 'self'`). The workspace sends the text of a diagram and gets the SVG back, as XML
  with its own size, and shows it as an image (`img-src data:`), where nothing in it can run. Its script is the one asset
  that may be fetched from no origin (`Cross-Origin-Resource-Policy: cross-origin`); it holds nothing but Mermaid.
- **Strict Mermaid:** `securityLevel: strict`, labels as SVG text, at most 50,000 characters.
- **Layouts:** Mermaid's ELK layout (elkjs, EPL-2.0 or GPL-3.0) is left out, as Mermaid's own tiny build does; a
  diagram that asks for it, or takes it by default, is laid out with dagre.
- A diagram Mermaid cannot read says why and shows its text. The theme of the workspace decides the colours.
- D2, which renders on the server, stays the way back should the frame ever not do.

## Where a copy came from

A text copied from a page of UnderDoc and pasted into a page notes its origin in the version of the block it lands in
(T-41, ADR-22): `copy of <page>#b3, version 4`, several places separated by `;`. A copy stays a copy: nothing ties it
to its source, the source does not count it as a use, and a change of the source does not reach it.

- **Copying** while reading (the marked text lies in blocks of one page of the database), in the editor (Ctrl+C or
  Ctrl+X) or with **Copy** of the marking bar puts, beside the text, the page and its blocks on the clipboard
  (`application/x-underdoc-origin`, `{page, blocks, text}`). In the editor only blocks as they are saved count; a copy
  from a block with changes not saved yet carries no origin.
- **Pasting** into the editor notes the stretch the text landed in, when the origin's text is the text pasted. Saving
  sends the places with the block the stretch lies in, `PUT …/blocks/{block}` or `POST …/blocks` with
  `{"copied": ["<page>#b3", …]}`.
- **The server** names only blocks of pages the writer sees, that are there, with the number of versions each has now,
  at most 20; anything else is left out, and nothing left is no origin. Text from anywhere else, or an origin beside
  another text, notes nothing.
- **The versions** (`GET /api/pages/{id}/versions`) show an origin with links to the pages the reader sees (`places`,
  each with `ref` and `title`); an address of a page the reader does not see reads "a page you do not see", for every
  origin that names addresses (`copy of`, `moved from`, `moved to`).
