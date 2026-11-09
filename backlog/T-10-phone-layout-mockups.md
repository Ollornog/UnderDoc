---
id: T-10
type: Task
title: Mock up the phone layout
status: offen
milestone: M-28
tags: [ui, mockup, phone]
created: 2026-11-07
---

The phone gets a layout of its own, not a squeezed desktop (ADR-17). Today the mockup only shrinks the columns at phone width.
The first build only keeps the phone in mind roughly, so this task can follow it. Mockups that can be clicked through, in two or
three variants where the answer is not obvious:

1. **Navigation:** how nav1, nav2 and nav3 become screens, and how the way back works.
2. **Tabs and windows:** how open documents are reached without a tab row, and whether a phone has more than one window at all.
3. **Head and tools:** what stays in sight above a document and what moves into a menu.
4. **Right click:** a long press opens the same menu, and what replaces dragging, for example sorting or moving into a collection.
5. **New and search:** where they sit, and how a quick note gets typed with one thumb.

**Done when:** the choice is recorded in ADR-16, and M-28 and M-14 are updated to match.

**Done before this, on 2026-11-09 (M-28):** the squeezed desktop works at phone width, nothing cut off and everything in
reach, proven by browser tests at 390×844 and 360×740 with touch input. nav2 and nav3 lie over the window at their full
width; on a touch screen the handle, the plus and the speech bubble of a block show faintly instead of on hover, with room
for them beside the page; a long press opens the right-click menu (point 4 in part). **Left for this task:** everything
above as a layout of its own. In particular nav1 still takes a column of its own (70 px of 360), the head of a document
wraps onto two or three rows, the block bar scrolls sideways instead of fitting, dragging (sorting, moving into a
collection, splitting off a tab) has no touch path yet, and a long press on iOS, which opens no menu of its own, is only
covered by the handler of the app, not tested on a real device.
