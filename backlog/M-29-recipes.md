---
id: M-29
type: Milestone
title: Recipes
status: offen
blocked_by: [M-28]
tags: [module, document-type]
created: 2026-11-08
---

A recipe for cooking is a document type of its own, asked for by the maintainer. Only an idea so far: the talk before this
milestone (ADR-14) settles the scope.

**To settle in that talk:**

- Which parts a recipe has: ingredients with amounts and units, steps, servings, times, a picture, tags such as the kind of dish.
- Whether the servings scale the amounts, and whether units convert.
- Whether recipes can be imported from a file or a site, and how without loading anything from that site when one is shown
  (ADR-16).
- Whether it is a module an administrator can switch off, like Bookmarks.

**Done when this is true:**

- A recipe is a document type, not a view of its own: it sits in collections like any document, with an ID, and opens in a tab
  (ADR-16, ADR-18).
- Its ingredients and steps are kept as structure, and Markdown stays the way in and out (ADR-19).
