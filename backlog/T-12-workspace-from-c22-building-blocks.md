---
id: T-12
type: Task
title: The workspace from C22's building blocks, the look as the pack underdoc
status: in-arbeit
milestone: M-14
tags: [workspace, c22, design]
created: 2026-11-22
---

The workspace was C22's old app workspace: the approved mockup of ADR-16 taken over whole, with classes of its own
(`.brandrow`, `.strip`, `.nav2`, `.dtab`, `.tools`, `.doc` …) under the root class `.c22-app-workspace`. C22 forbids that
now (its ADR-2, rule of 2026-11-21): **structure only from C22 building blocks; an app's own look only as its own layout
pack, named after the repository.** C22 rebuilt the mockup from `.workspace` and its parts and moved its look into the pack
`underdoc` (C22 T-19); the old block goes once UnderDoc has switched (C22 T-20).

## Steps

1. Vendor C22 with the pack `underdoc` (`c22-underdoc.css` on every page instead of `c22-vega.css`); the class check also
   reads the scripts of the workspace and the editor.
2. The frame: top row, nav1 to nav3 with the list, windows with their tabs, the head of a window, the document, the status
   line (C22's replacement list, core).
3. The editor and its parts: bar, block, handle, slash menu, embeds, parts, marked text (`.cm-ud-plain` goes).
4. Menus, panels and the other views: New, the command menu, context menus, the gear, comments, facts, settings,
   profile, administration, files, resources, search, mail, calendar, tasks, boards, the assistant.
5. Dialogs and pages outside the workspace: rights (steps, split badges, a grouped combobox, timeline, retention with
   its unit), passwords, the page behind a link, the table view of a spreadsheet (`.tabs`, `.table[data-variant=grid]`).
6. The root class `.c22-app-workspace` goes; what is left of own CSS carries no look.

## State

- Step 1 (2026-11-22): the pack `underdoc` on every page, the class check reads the scripts (#126).
- Step 2 (2026-11-22): the frame is `.workspace` with `data-behaviour="app"` (C22 at `70d7da3`): top row, nav1 to nav3
  with `data-step` and `--workspace-nav3`, the tree rows of every list in nav3, `.workspace-windows` with one
  `.workspace-window` per window (`data-focus` with several), the head of a window, `article.typeset[data-width]`, the
  status line, the drop zones and the mark of a dragged tab. Menus, drawers, the editor and the other views keep their
  old form for steps 3 and 4; until step 3 the article keeps the class `doc` of the old block for its blocks, and on a
  phone the section gives the handles their room (`ps-9`). Open in C22: a blank space between entries of a list, a
  window without tabs (solo), open and kept open apart (thin and thick line), the typeset flow inside block wrappers,
  the head of a window and the room for handles on a phone.

## Done when

UnderDoc emits none of the classes of C22's replacement list (C22 T-19), the class check passes against a stylesheet
without the old block, the browser tests pass, and pictures before and after (light, dark, phone) show no unexplained
difference.
