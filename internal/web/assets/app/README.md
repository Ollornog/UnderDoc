# The workspace in the browser

`demo.js` draws the workspace, `theme.js` sets light or dark before the page shows, `blockdrag.js` drags a block of a page by its handle, `parts.js` shows the boundary of a part while
reading (ADR-22), `timetracking.js` lays the frames of the time tracking over their windows (docs/time-tracking.md), `kinds.js` draws
the kind of a page, its writing rules and the hints of AI changes (docs/page-kinds.md), `diagrams.js` has Mermaid draw
diagrams in a sandboxed frame and shows them as images (docs/parts.md#diagrams).
All are UnderDoc's own (MIT).
`editor/` holds the web editor, built from `web/editor` (docs/editor.md): its own code is MIT, and `editor/LICENSES.txt`
has the licence of every package bundled into it. `mermaid/` holds the script of that frame, built from `web/mermaid`
with Mermaid and the packages it brings, and `mermaid/LICENSES.txt` their licences.

| Part | From | Licence |
|---|---|---|
| The gear icon in `demo.js` (`settings`) | Lucide 1.25.0 | `licenses/Lucide-LICENSE.txt` (ISC) |
