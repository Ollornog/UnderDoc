---
id: ADR-5
type: Decision
title: One instance per customer
status: erledigt
tags: [architecture, security]
created: 2026-11-06
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
