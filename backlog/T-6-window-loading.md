---
id: T-6
type: Task
title: Choose how windows load their documents
status: offen
milestone: M-15
blocked_by: [T-5]
tags: [ui, security]
created: 2026-11-06
---

Compare frames from the same origin with fragments swapped into one page, under the strict CSP and without a JavaScript
toolchain. Look at independent scrolling, IDs that clash when a document is open twice, keyboard use, memory with four
windows and what the later editor (M-8) needs.

**Done when:** a small prototype of both has been measured, and the choice is recorded in a decision.
