---
id: ADR-21
type: Decision
title: The web editor is CodeMirror 6 with a live preview
status: erledigt
tags: [editor]
created: 2026-11-09
---

## Context

M-8 asks for writing inline, as in Typora: the page looks as it is shown, and the Markdown of a passage appears only where
the cursor is. Pages live in the database as blocks of Markdown (ADR-19), and the Markdown round trip must keep every
directive, such as `::embed{…}` and `[[links]]` (concept, risk 4). Nothing may be loaded from third parties, and the strict
CSP stays as it is.

## Options

A trial page put three candidates side by side with the same content of real pages, typed Markdown shortcuts and a round
trip test (2026-11-09, `/dev/editor-trial`):

1. **Milkdown (Crepe preset):** the most complete interface out of the box (toolbar, slash menu, tables). WYSIWYG only, no
   Markdown at the cursor. About 264 KiB gzip plus 589 KiB on demand, 163 packages. The round trip lost the alt text of
   images and reformatted tables; under the CSP only inside a shadow root.
2. **TipTap, MIT parts only:** mature, 157 KiB gzip, 38 packages; its paid extensions and cloud were left out. WYSIWYG only.
   The round trip escaped `[[wiki links]]` and reformatted tables; no slash menu without building one.
3. **CodeMirror 6 with a live preview of our own:** the text stays Markdown; away from the cursor it looks rendered, at the
   cursor the Markdown shows. 105 KiB gzip, 11 MIT packages. Every typed shortcut converted, and the round trip was
   identical, since nothing is converted at all. Toolbar, slash menu and table tools have to be built.

A rich-text engine of our own from scratch was ruled out: selection, undo, paste, input methods and phones on
`contenteditable` take years. An engine of our own would be built on ProseMirror at best.

## Decision

CodeMirror 6 with a live preview, chosen by the maintainer on 2026-11-09. The other two stay on the trial page for
comparison until M-8 closes. **Done on 2026-11-17:** M-8 closed, and the trial page went with its packages; it lives on in
the history of the repository, last in commit `d9b5bdb` (`web/editor-trial`, `internal/browsertest/editor_trial_test.go`).

- **One editor per page.** The page is written as one document; each block keeps its ID through a marker line before it,
  `<!-- block bN -->`, the same as in the export (docs/export.md). The editor hides the markers and shows the block handle
  there. Saving compares the blocks with what is stored and adds, changes, moves or deletes blocks, so versions, embeddings
  and comments stay per block.
- **Every button and the slash menu write plain Markdown** (M-8), so the round trip stays whole by construction.
- **The JavaScript is built from a lockfile with esbuild** and served by UnderDoc itself; the pipeline builds it and checks
  that the shipped bundle comes exactly from the lockfile. Licences of all bundled packages ship with it.

## Consequences

- The live preview, the toolbar, the slash menu and the table tools are ours to keep; they are small and covered by tests.
- Collaboration later (M-10) can use Yjs with its MIT binding for CodeMirror.
- The block editing of M-28 (one Markdown field per block) goes once the editor carries everything it did.
