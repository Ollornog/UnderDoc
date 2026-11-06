---
id: T-3
type: Task
title: Render Mermaid under a strict CSP
status: offen
milestone: M-2
tags: [mermaid, security]
created: 2026-11-06
---

Mermaid generates inline styles, which may conflict with a strict CSP. Test nonces and rendering in an isolated frame. `unsafe-inline` for the whole app is not an option. D2 is the fallback, as it renders on the server.

**Done when:** a diagram renders with the production CSP, with a browser test.
