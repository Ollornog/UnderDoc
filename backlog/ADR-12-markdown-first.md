---
id: ADR-12
type: Decision
title: Start with Markdown only
status: erledigt
tags: [planning]
created: 2026-11-06
---

## Context

The first milestone combined the wiki, Excel, Samba, versions, permissions and search. That is too much to finish quickly
and too much to learn from at once.

## Options

- **Everything in the first milestone:** tests the biggest risk early, but delays anything usable.
- **Markdown only first:** usable within a short time and gives the app its skeleton, while the Excel risk stays open for longer.

## Decision

M-1 is a Markdown wiki without a database. Excel, Samba and versions move to M-2. The measurement of how Office saves (T-1)
is independent of the code and can run at any time, so the risk does not have to wait.

## Consequences

- M-1 keeps its index in memory, so PostgreSQL (ADR-2) arrives with M-2.
- Until M-2, the app has no permissions of its own, so it must not hold anything a signed-in user may not read.
