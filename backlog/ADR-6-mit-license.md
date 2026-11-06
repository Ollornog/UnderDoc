---
id: ADR-6
type: Decision
title: MIT license
status: erledigt
tags: [license]
created: 2026-11-06
---

## Context

The project is open source. Paid hosting and support are offered separately and do not affect the repository.

## Options

- **MIT:** simplest for companies, but others may build closed products from it.
- **Apache-2.0:** like MIT with a patent grant and a few more duties.
- **AGPL-3.0:** changes stay open also for web use, but some companies avoid it.

## Decision

MIT.

## Consequences

- Anyone may build a closed product from it, which is accepted.
- Dependencies must fit, and license scanning belongs in the pipeline before the first JavaScript dependency.
