---
id: ADR-14
type: Decision
title: One topic at a time, built as modules
status: erledigt
tags: [process, architecture]
created: 2026-11-06
---

## Context

The wish list keeps growing: windows, a calendar, forms, reviews through Git, website display. Taken together that is
a wiki, a list tool, a file server and part of a code forge. Built side by side, none of it would ever be finished, and
every part would depend on every other.

## Options

- **Build in parallel:** visible progress everywhere, but nothing finished and many half-open seams.
- **Fewer features:** cleaner, but the wishes are real.
- **One topic at a time, each a module on a small shell:** slower to look complete, but every step stays usable.

## Decision

The third option.

- **One milestone is in progress at a time.** It closes with its acceptance before the next one starts. Bugs and small
  fixes may interrupt.
- **Mockups come before UI work.** Ways of working are chosen from variants that can be clicked through, and the choice
  is recorded in a decision.
- **Modules plug into the shell.** A module adds its tab, its sections in the facts panel and its routes through one
  small interface. A module with nothing to show adds no tab.
- **Modules do not reach into each other's data.** They use each other's public interfaces.
- **The order of work is in [the concept](../docs/concept.md#order-of-work).** Milestone numbers are IDs, not an order.

## Consequences

- The shell (M-14) comes before the modules that plug into it, even though the technical risk sits in M-2.
- A wish that arrives during a milestone becomes a backlog entry, not a part of the current work.
- Each later milestone lists what it adds to the facts panel and to the tabs.
