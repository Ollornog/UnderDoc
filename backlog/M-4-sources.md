---
id: M-4
type: Milestone
title: External sources with cache
status: offen
tags: [sources]
created: 2026-11-06
---

Data from other systems appears as native tables.

**Done when this is true:**

- PostgreSQL, MariaDB, MSSQL, Baserow, NocoDB, REST and CSV work as sources.
- Queries run on the server only, with read-only accounts, time and row limits and a target allowlist.
- Every view shows how old its data is and falls back to the last good state.
- Cached data is searchable.
- Adds the document types database and database view. Sources and connections appear in the Resources tab, and the facts panel
  adds the databases and views a document uses.
