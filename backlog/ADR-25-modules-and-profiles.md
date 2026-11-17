---
id: ADR-25
type: Decision
title: Every module can be switched, by the administrator and by each user
status: erledigt
tags: [architecture, modules]
created: 2026-11-16
---

## Context

One person needs only the documents, another documents and mail, a third everything. UnderDoc runs privately and in small
companies (the maintainer, 2026-11-16), and hosters offer it to many tenants (ADR-23).

## Decision

- **Every module can be switched on and off:** documents, files, lists, sources, search engines, mail, calendar, private area
  with time and leave, bookmarks, news, recipes, forms, websites, network, drawing, AI, review with a forge.
- **The administrator switches per tenant** in the interface. A hoster sets the defaults and what a tenant may switch on
  through the environment (`UNDERDOC_MODULES`).
- **Each user can hide** a module switched on for them.
- **Switching off never deletes data,** and a module switched off leaves no entry, no route and no job behind.
- Modules hook in through the one small interface of M-14 (document types, views, metadata sections, search sources).

## Consequences

- Each module has its own tests with the module on and off.
- The private area serves single users and companies: time and leave can run purely privately, or with the full company
  function of M-24, which a company switches on itself.
- Built on 2026-11-17 ([docs/modules.md](../docs/modules.md)): Documents is the first module; hiding is not switching
  off, so what a user hides stays reachable by address and embedding. The modules hook into the interface of M-14 once
  it exists; until then `module.All`, the guard of the routes and `web.ModuleOn` for jobs are that interface.
