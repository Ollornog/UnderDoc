---
type: Task
title: 'Where a pasted text came from: the origin in the version of the block it lands in'
status: offen
points: 2
id: T-41
milestone: M-3
created: '2026-11-24'
---
# T-41 — Where a pasted text came from: the origin in the version of the block it lands in

Open in M-3 since 2026-11-19. When text is copied from one page and pasted into another through the clipboard, the version of the block it is pasted into records where it came from (object and block id, version), so the history shows the origin without tying the copy to its source (a copy stays a copy, ADR-22). Browser test: copy a passage, paste it, the version shows its origin; pasting from outside UnderDoc records nothing.
