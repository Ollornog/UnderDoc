---
id: ADR-5
type: Decision
title: One instance per customer
status: verworfen
tags: [architecture, security]
created: 2026-11-06
superseded_by: ADR-23
---

## Context

The product will run for several organizations.

## Options

- **Tenants in the code:** cheaper to run, but one bug can show data across customers.
- **One instance per customer:** more instances, but no such class of bug.

## Decision

One instance per customer, no tenants in the code.

## Consequences

- Updates must be automated across instances.
- Spaces inside an instance are libraries, not tenants.

**Replaced on 2026-11-16 by ADR-23:** one tenant stays the default mode, and a mode with many tenants joins it.
