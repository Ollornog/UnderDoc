---
id: T-12
type: Task
title: The workspace from C22's building blocks, the look as the pack underdoc
status: erledigt
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
- Step 2 (2026-11-22): the frame is `.workspace` with `data-behaviour="app"` (C22 at `4bc20af`): top row, nav1 to nav3
  with `data-step` and `--workspace-nav3`, the tree rows of every list in nav3, `.workspace-windows` with one
  `.workspace-window` per window (`data-focus` with several), the head of a window, `article.typeset[data-width]`, the
  status line, the drop zones and the mark of a dragged tab. Menus, drawers, the editor and the other views keep their
  old form for steps 3 and 4; until step 3 the article keeps the class `doc` of the old block for its blocks, and on a
  phone the section gives the handles their room (`ps-9`). Open in C22 (the bridges go once it has them: `doc`,
  `ps-9`, `--workspace-step-1` on a phone, `size-3`): a blank space between entries of a list, open and kept open
  apart (thin and thick line), the typeset flow inside block wrappers, the room for handles on a phone, the width of
  nav1 on a phone, the icon in the head of a section, the size of the avatar.
- Step 3 (2026-11-22): the editor and its parts (C22 at `ec39f36`): `.editor`, `div[data-block]` with
  `.editor-handle`, `.command`, `.item` questions, `[data-used]`, `[data-part]`, `figure.editor-embed`, the card of the
  ways, quick notes as cards, `.comment-anchor`, `pre.code-block`; the bridges of step 2 are gone (`doc` on block pages,
  `ps-9`, `--workspace-step-1`, `size-3`, the pin, `hr.w-full` and `span[data-fill]` as lines and spaces). The gaps the
  editor found in C22 (flow inside a wrapper and an embedding, to-dos in typeset, the active handle in a line editor, the
  frame of a used block over sibling lines, the gap of a held block, continued frames of a part, touch on handles and
  bubbles, the bubble within the pack's padding) C22 closed in #145; the bridges UnderDoc had for them meanwhile are gone.
- Step 4 (2026-11-22): menus, drawers, panels and the other views (C22 at `ec39f36`, recipes `workspace-views.html` and
  `workspace-more.html`): `.context-menu`, `.dropdown-menu`, `.command`, the popover of New, `.toaster`, the drawer
  `aside[data-drawer]` for the gear, the comments (`ol.timeline`) and the assistant (`.bubble`), a `.card` in nav3 for
  profile, settings, administration and Rights, `.item` lists for mail, news, to-dos, hits and modules,
  `table.month-sheet`, `.board`, `.filter-bar`, and for content without blocks `.alert`, `.table`, `.empty`,
  `figure.card`; the class `doc` is gone, every menu hangs inside `.workspace`. Bridged with C22's utilities until C22
  has them: the current hit of Find (`mark[aria-current]`), the element whose comments are open
  (`data-comment-focus`), the size of an icon in the head of a drawer. Open in C22: menu entries of `.context-menu` at the
  pack's size, the drawer below the head of the window as in the mockup.
- After step 4 (2026-11-22): C22 vendored at `bad252a` (C22 #149 closes the gaps of step 4); the bridges are gone:
  `mark[aria-current]`, `.comment-anchor[data-comment-focus]`, the icon in the head of a drawer, context menus at the pack's
  size, the drawer below the head, modules as switches.
- Steps 5 and 6 (2026-11-22): the rights dialog (entries, icons per kind, `.badge-split` from above, a ghost cross to take
  back), a new collection as `form[data-steps]` with `ol.steps`, the passwords dialog as entries, the log as
  `ol.timeline[data-size=sm]`, the table view as `.tabs` and `table.table[data-variant=grid][data-sticky=sheet][role=grid]`,
  tables in pages with `data-type="number"`, the share page after C22's `public-page.html`. `c22.js` is loaded (steps);
  `data-behaviour="app"` keeps it out of the workspace (a test spies on it). The root class `.c22-app-workspace` is gone;
  the class check reads the server's classes too and passed against a C22 stylesheet without the old block (T-20,
  `f078f71`), as did the browser tests. Left in C22 (reported): the grouped `.combobox` without Basecoat, a current row of
  a grid, the nav3 rule for a `header` that also catches the head of a timeline entry in a card, the live preview of the
  editor (`editor.css` keeps its look until `.editor` has one).
- Done (2026-11-22): C22 vendored at `a023d57` (C22 #153 closes the gaps of steps 5 and 6, #154 removes the old app
  workspace, T-20). The live preview is C22's `data-md`, `editor.css` keeps no look; the share choice is the grouped
  `.combobox` with Basecoat's core and combobox alone (a test proves the workspace's menus stay the app's); the row of a
  hit in a sheet is `aria-current`. Two bridges are left for C22 (reported): the list of a combobox cannot leave a section
  that scrolls, so the steps of a new collection do not scroll within the dialog; a heading in `.editor` has no room
  around it as in `.typeset`, so `editor.css` gives it that room.
- After done (2026-11-22): C22 vendored at `6d8a2ae` (C22 #155): the two bridges are gone. The list of the share
  combobox is `data-popover-fixed` and leaves the section, so the steps of a new collection scroll within the dialog
  again; a heading in `.editor` has its room from C22. Taken over as well: the drawn table in its reading box with its
  frame, and no lines of empty groups while one types in the combobox.

## Done when

UnderDoc emits none of the classes of C22's replacement list (C22 T-19), the class check passes against a stylesheet
without the old block, the browser tests pass, and pictures before and after (light, dark, phone) show no unexplained
difference.
