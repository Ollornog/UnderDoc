---
type: Task
title: 'Data exchange: import foreign tables, print a view, export tables'
status: offen
sprint: S5
points: 8
id: T-14
milestone: M-17
created: '2026-11-22'
---
# T-14 — Data exchange: import foreign tables, print a view, export tables

UnderDoc exchanges data, it is not a backup (decided 2026-11-21 with the maintainer; a full backup export was dropped).

- Import foreign tables and other data into UnderDoc.
- Print a view, or save it as PDF.
- Export a table as CSV, XLSX or ODS.
- An export carries no permissions: persons, groups and roles stay inside.

Starts with an ADR. Writing new spreadsheet files belongs to gridport (T-13).
