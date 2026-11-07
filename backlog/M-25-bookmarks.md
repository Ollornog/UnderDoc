---
id: M-25
type: Milestone
title: Bookmarks
status: offen
blocked_by: [M-5]
tags: [module, people]
created: 2026-11-07
---

Links to the outside sit next to the documents, as a module an administrator can switch off. Only an idea so far: the talk before
this milestone (ADR-14) settles the scope.

**To settle in that talk:**

- Bookmarks per user only, or also shared ones in a collection.
- Whether a bookmark can be placed in the workspace like a document, with tags and children.
- Whether a link can be checked for being dead, and how without contacting the site for every view.

**Done when this is true:**

- Bookmarks have a view in nav1, with folders in nav2 and the links in nav3, and each opens in a tab like a document.
- The HTML file that every browser exports imports with its folders, and links already there are skipped.
- Nothing is loaded from a linked site to show a bookmark, no preview and no icon (ADR-16).
- Bookmarks export to the same HTML format.
- Switched off, the module leaves no entry behind, and no data is touched.
