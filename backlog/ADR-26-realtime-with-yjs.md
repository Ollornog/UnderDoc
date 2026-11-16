---
id: ADR-26
type: Decision
title: Editing together with Yjs and a small Node companion
status: erledigt
tags: [editor, realtime, architecture]
created: 2026-11-16
---

## Context

M-10 asks for several people typing in the same page at once. The editor is CodeMirror 6 (ADR-21), which has an MIT binding
for Yjs. There is no mature Yjs server in Go.

## Options

- **Block by block in Go:** presence and live updates of other blocks, a soft lock on a block in use. No new runtime, but no
  typing together in one block.
- **Yjs with a Node companion:** typing together everywhere, one more runtime.

## Decision

Yjs with a Node companion, chosen by the maintainer on 2026-11-16.

- The companion is Hocuspocus (MIT) on y-websocket, built from a lockfile like the editor bundle and shipped in the same
  image. UnderDoc starts and watches it, and it listens only locally.
- **UnderDoc stays the truth:** the browser connects to UnderDoc, which checks the session and the right to edit before it
  passes the connection on. The companion loads a page from UnderDoc and stores it back through UnderDoc, which turns the text
  into blocks as autosave does today, so versions, embeddings, comments and the parts of ADR-22 keep working.
- Presence shows who is on the page and where their cursor is.

## Consequences

- The pipeline carries the Node part like the editor bundle: lockfile, build, licences, a check that the image holds it.
- Without the companion, editing falls back to saving alone, with the reload on a conflict of today.
