# Editor trial (M-8)

Three candidates for the web editor of M-8, side by side on `/dev/editor-trial` (with `UNDERDOC_DEV_UI=1` or for
administrators, behind the gate like every page). CodeMirror won (ADR-21, built in `web/editor`); the trial stays for
comparison until M-8 closes. Each starts with the same Markdown fixture (`src/fixture.js`), made of what UnderDoc pages hold.
"Markdown out" shows what the editor gives back and a line diff against the fixture. This is a trial: after the choice,
the chosen editor is built properly.

| Column | Candidate | Source |
|---|---|---|
| 1 | Milkdown with its Crepe preset (ProseMirror + remark) | `src/milkdown.js` |
| 2 | TipTap, MIT parts only, with the official `@tiptap/markdown` | `src/tiptap.js` |
| 3 | CodeMirror 6 with a live preview written here (decorations) | `src/codemirror.js` |

## Build

```sh
cd web/editor-trial
npm ci --ignore-scripts
npm run build
```

`build.mjs` bundles with esbuild into `internal/web/assets/app/editor-trial/` (ES modules, one build per entry; what an
editor loads on demand lands in `<entry>-chunks/`). It also writes `meta.json`: the version and licence of every bundled
package and the gzipped size of every bundle, which the page shows under each editor. The built files are not committed:
`scripts/build-js.sh` and the Dockerfile build them from the lockfile wherever the Go binary that embeds them is built.
Nothing is loaded from elsewhere.

The versions are exact, stable, and were at least two weeks old when installed: `npm install --save-exact
--before=2026-10-26` (the cutoff applies to the transitive packages too).

## Licences

All bundled packages are MIT, except `dompurify` (MPL-2.0 OR Apache-2.0, pulled in by Crepe). `esbuild` (build only) is
MIT. The full list per bundle is in `meta.json`, and `LICENSES.txt` next to the bundles has the licence text of every
bundled package.

## The strict CSP

The CSP stays as it is (`style-src 'self'`, no inline style). What the candidates need for it:

- **TipTap** adds its base styles as a `<style>` element. `injectCSS: false`, and the styles are in `src/tiptap.css`.
- **CodeMirror** (and Crepe's code blocks, which are CodeMirror) write their base styles with style-mod as a `<style>`
  element into a document. In a shadow root style-mod uses a constructed style sheet, which the CSP does not block. So both
  editors live in a shadow root, with their stylesheet linked inside it. `?milkdown=light` mounts Milkdown in the page
  instead, to see the violation and the unstyled code block.

## The browser test

`go test -tags browser -run TestEditorTrial ./internal/browsertest/` loads the page, types Markdown with real keys into
each editor, and logs a comparison table; the same as JSON goes to a file the log names. `BROWSER_SHOTS=<folder>` saves
pictures. It fails only when the page or the harness breaks, not on what an editor cannot do.
