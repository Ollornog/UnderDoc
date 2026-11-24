---
type: Task
title: 'Where a pasted text came from: the origin in the version of the block it lands in'
status: erledigt
points: 2
id: T-41
milestone: M-3
created: '2026-11-24'
---
# T-41 — Where a pasted text came from: the origin in the version of the block it lands in

Open in M-3 since 2026-11-19. When text is copied from one page and pasted into another through the clipboard, the version of the block it is pasted into records where it came from (object and block id, version), so the history shows the origin without tying the copy to its source (a copy stays a copy, ADR-22). Browser test: copy a passage, paste it, the version shows its origin; pasting from outside UnderDoc records nothing.

**Closed on 2026-11-24:** copying while reading, in the editor and with Copy of the marking bar puts the page and its
blocks beside the text on the clipboard; pasting notes the stretch, and saving it writes `copy of <page>#b3, version 4`
into the version of the block it lands in. Only blocks the writer sees and that are there are named; the versions hide
pages the reader does not see. Browser test `TestAPastedTextNotesWhereItCameFrom`, API test
`TestAPasteNotesWhereItCameFrom` ([docs/parts.md](../docs/parts.md#where-a-copy-came-from)).
