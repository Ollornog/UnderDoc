---
id: ADR-1
type: Decision
title: Build our own instead of forking a wiki
status: erledigt
tags: [architecture, license]
created: 2026-11-06
---

## Context

A self-hosted wiki is needed for company use and for paid hosting for customers. What sets it apart is the data model: files as truth, values, snippets, sources.

## Options

- **Fork Outline:** BSL 1.1, internal use allowed, but a commercial Document Service for third parties is forbidden, and the current version changes to Apache-2.0 only in 2030.
- **Fork Docmost:** AGPL-3.0, with enterprise parts such as SSO under a separate license. No plugin system, so every upstream release means patching again.
- **Own code on existing libraries:** free choice of license, but editor, real-time and permissions are expensive.

## Decision

Own code. A fork would have to rebuild exactly the data model, while the expensive editor can come from existing libraries and can wait until the end.

## Consequences

- No code, texts, assets or translations are taken from those projects.
- The product must be useful before it has an editor, see ADR-4.
