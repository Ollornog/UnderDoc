# The workspace in the browser

`demo.js` draws the workspace, `theme.js` sets light or dark before the page shows, `signin.js` drives passkeys on the
pages of signing in, `blockdrag.js` drags a block of a page by its handle, `parts.js` shows the boundary of a part while
reading (ADR-22), `timetracking.js` lays the frames of the time tracking over their windows (docs/time-tracking.md).
All are UnderDoc's own (MIT).
`editor/` holds the web editor, built from `web/editor` (docs/editor.md): its own code is MIT, and `editor/LICENSES.txt`
has the licence of every package bundled into it.

| Part | From | Licence |
|---|---|---|
| The gear icon in `demo.js` (`settings`) | Lucide 1.25.0 | `licenses/Lucide-LICENSE.txt` (ISC) |
