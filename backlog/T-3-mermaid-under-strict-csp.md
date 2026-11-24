---
id: T-3
type: Task
title: Render Mermaid under a strict CSP
status: erledigt
points: 3
milestone: M-3
tags: [mermaid, security]
created: 2026-11-06
---

Mermaid generates inline styles, which may conflict with a strict CSP. Test nonces and rendering in an isolated frame. `unsafe-inline` for the whole app is not an option. D2 is the fallback, as it renders on the server.

**Done when:** a diagram renders with the production CSP, with a browser test.

**Closed on 2026-11-24:** Mermaid 12.3.0, bundled from `web/mermaid`, draws in a sandboxed frame without an origin of its
own (`sandbox allow-scripts`, its own policy with inline styles but no inline script and no connection,
`frame-ancestors 'self'`); the workspace shows the SVG as an image and keeps its policy. Values and parts embedded in a
diagram are drawn with their text. Browser tests `TestADiagramDrawsUnderTheStrictPolicy` and `TestABrokenDiagramSaysSo`
([docs/parts.md](../docs/parts.md#diagrams)). D2 was not needed.
