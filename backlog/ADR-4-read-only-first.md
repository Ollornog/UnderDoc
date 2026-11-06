---
id: ADR-4
type: Decision
title: Read-only first
status: erledigt
tags: [planning]
created: 2026-11-06
---

## Context

Editor, inline editing and real-time are the largest parts and the least certain ones.

## Options

- **Editor first:** feels like a product sooner, but delays what sets it apart.
- **Read-only first:** useful early, since editing happens at the source.

## Decision

Read-only first. Exception: native lists have no other source, so they get web editing from the start (ADR-8).

## Consequences

- Whoever cannot reach the files can only read until M-7.
- The JavaScript toolchain is only needed from M-8.
