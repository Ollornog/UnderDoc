# Vendored design system

Build output of [C22](https://github.com/Ollornog/C22), which does not track its compiled style packs. Refresh it with
`scripts/vendor-c22.sh`, never by hand. The commit it came from is in `VERSION`.

| File | From | License |
|---|---|---|
| `css/c22-vega.css` | C22, compiled with Tailwind CSS and Basecoat | `licenses/C22-LICENSE`, `licenses/Tailwind-LICENSE`, `licenses/Basecoat-LICENSE` |
| `js/basecoat.all.min.js` | Basecoat, through C22 | `licenses/Basecoat-LICENSE` |
| `js/c22.js`, `js/workspace-boot.js` | C22: the behavior of its components, among them the workspace | `licenses/C22-LICENSE` |
| `js/app-workspace-drag.js` | C22: dragging entries of nav2 and nav3; the app decides what may go where | `licenses/C22-LICENSE` |
| `fonts/inter.woff2` | Inter, through C22 | `licenses/Inter-OFL.txt` |
