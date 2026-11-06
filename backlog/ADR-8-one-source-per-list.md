---
id: ADR-8
type: Decision
title: Every list has exactly one source
status: erledigt
tags: [lists]
created: 2026-11-06
---

## Context

Customer and contact lists exist as Excel files today and should become more reliable without creating two truths.

## Options

- **Two-way sync between Excel and the database:** comfortable, but conflicts are unsolvable in practice.
- **One source per list:** clear, but moving to a native list is a decision.

## Decision

A list is either native, an Excel file or an external source. Moving from Excel to native is a one-time import. Exports to the share are marked read-only copies with no way back.

## Consequences

- Native lists need web editing from the start.
- Deliberately no CRM features.
