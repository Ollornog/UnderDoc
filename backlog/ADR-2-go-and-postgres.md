---
id: ADR-2
type: Decision
title: Go and PostgreSQL
status: erledigt
tags: [architecture]
created: 2026-11-06
---

## Context

The server runs one instance per customer, talks to many sources and should be simple to operate.

## Options

- **Go:** one binary, strong standard library, pure Go database drivers.
- **Rust:** catches memory and thread errors at compile time and needs less memory, but it is slower to write and to compile,
  needs more packages for a web server, has no single Excel library that both reads and writes, and the pipeline does not
  fully carry it yet. Its speed helps little here, as the time goes into the database, files and the network.
- **Python:** familiar, but needs a runtime and is weaker for many parallel sources.
- **TypeScript:** would match the editor, but brings a larger dependency tree to the server.
- **SQLite instead of PostgreSQL:** simpler, but weaker for many writers and German full-text search.

## Decision

Go for the server, PostgreSQL as the only database. Server-rendered HTML with HTMX, JavaScript only where it earns its place.

## Consequences

- No support for a second database.
- The editor brings a JavaScript part, and possibly a Node sidecar for real-time editing.
