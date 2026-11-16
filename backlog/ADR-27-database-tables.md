---
id: ADR-27
type: Decision
title: Native tables are rows in JSONB with field definitions, and Markdown tables convert by a button
status: erledigt
tags: [lists, architecture]
created: 2026-11-16
---

## Context

UnderDoc gets a table editor of its own in the spirit of Baserow (M-6): typed fields, links between tables, views with filters,
rows edited in the web. Views are embedded in pages, and writing into them can be switched on (M-9). Tenants are part of the
data model (ADR-23).

## Options

- **A real SQL table per user table,** as Baserow does: native types and constraints, fast at millions of rows, readable by
  outside tools. But every change of a field is DDL at runtime, a change of type rewrites and locks the table, row-level security
  has to be set per table, a hoster ends up with tens of thousands of tables, and history needs triggers per table.
- **Rows as JSONB with field definitions as data:** changes of fields are data, in one transaction and undoable, one table and
  one rule for tenants, one history for all. Types are checked by the app, and large tables need indexes per field.

## Decision

JSONB with field definitions, confirmed by the maintainer on 2026-11-16.

- Field types: text, number, date, single and multiple select, check box, rating, person, file, link to another table, lookup
  and formula. Views: grid, kanban, calendar, gallery and form, each with filter, sort and grouping. Every row has a history and
  comments.
- Links between rows live in a table of their own, so they stay relational.
- When a view filters or sorts by a field, UnderDoc creates an index on that field with `CREATE INDEX CONCURRENTLY`. No other DDL
  happens at runtime.
- For outside tools, UnderDoc offers a readable SQL view per table, within the tenant.
- The limit is measured and documented. The aim is 200,000 rows per table that still feel quick. Larger data stays in its own
  database and is connected as a source (M-4).
- **A Markdown table stays Markdown** until it is converted by a button. The block keeps its ID and becomes an embedding of the
  new table in the same place, so every embedding elsewhere shows the table. Converting back works as long as no feature of a
  native table is used. From about 20 rows, or when it is embedded elsewhere, the editor offers the conversion.
- **Views of sources** (M-4) show the cached state at once with its age, refresh in the background after loading and swap the
  new state in. Per source and view, this can be set to always live (wait for fresh data) or cache only (refresh on a schedule),
  with a maximum age after which a warning shows.
- **Writing back** (M-9) is switched per source, per view and per column. Native tables follow the rights of the table, not of
  the page they are embedded in. External databases need a write account of their own, a known primary key, typed checks, a
  check that the old value is still there before writing, and a log of every change. Nothing changes their structure.
