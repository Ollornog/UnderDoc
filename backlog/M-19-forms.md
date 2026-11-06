---
id: M-19
type: Milestone
title: Forms and simple surveys
status: offen
blocked_by: [M-6]
tags: [lists, forms]
created: 2026-11-06
---

Only an idea so far. Check again whether it is needed before building it.

**Done when this is true:**

- A form on a native list adds rows to that list, using its field types.
- A simple survey is a form without an existing list. The app creates a plain list behind it, so answers follow the same
  rules as any list (ADR-8).
- A form can be embedded in a page.
- Forms work for signed-in users or, with a token, publicly. Public forms have limits against abuse.
- Forms on external databases only where writing back is enabled for that source (M-9).
- Personal data in answers can be deleted, and how long it is kept is configurable.
