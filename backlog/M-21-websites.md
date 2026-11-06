---
id: M-21
type: Milestone
title: Showing websites
status: offen
tags: [ui, security]
created: 2026-11-06
---

Websites can be shown in a page or a window. T-8 decides which kinds, because this touches the rule that nothing is
loaded from third parties.

**Done when this is true:**

- The kinds chosen in T-8 work under the production CSP, with a browser test.
- Nothing from a third party loads until an administrator has allowed its origin and the reader has clicked.
- Uploaded HTML cannot run with the rights of the app.
