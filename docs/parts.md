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

## Still to come in M-3

Deleting something in use with its three ways, the placeholder, moving with redirects, the boundaries in reading and in
the editor, marking from the editor, values in the Resources tab: see ADR-22 and M-3.
