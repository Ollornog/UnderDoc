---
id: M-26
type: Milestone
title: News from subscribed feeds
status: offen
blocked_by: [M-5]
tags: [module, people]
created: 2026-11-07
---

Entries from feeds a user subscribes to, as a module an administrator can switch off. Only an idea so far: the talk before this
milestone (ADR-14) settles the scope.

**To settle in that talk:**

- RSS and Atom only, or also pages that change.
- How often the server fetches, and whether an administrator limits which addresses it may reach.
- Whether an entry can be kept as a document or sent to a collection.

**Done when this is true:**

- News has a view in nav1, with the feeds in nav2 and the entries in nav3, read and unread, and each entry opens in a tab.
- Every user has their own subscriptions.
- The server fetches the feeds, so the browser loads nothing from a third party.
- Switched off, the module leaves no entry behind, and no data is touched.
