---
id: T-7
type: Task
title: Decide how reviews work, on Git or in the app
status: offen
milestone: M-20
tags: [review, git]
created: 2026-11-06
---

- **Git only:** a reviewed folder is a Git-backed folder, and review and issues stay on the forge. There is one review
  mechanism and nothing to rebuild, but reviewers need a forge account, and Excel files show no useful diff.
- **In the app:** proposals and approval live in UnderDoc. This works for every file type and every user, but it rebuilds
  part of a forge.
- **Both:** the most reach and the most work.

The starting recommendation is Git only, adding approval in the app if people without a forge account need it. Issues are
not rebuilt in any option. Also settle how Git-backed folders fit ADR-13 (Git stays their source, like a folder source),
how they fit real-time editing (M-10) and which forges come after GitHub.

**Done when:** the choice is recorded in a decision, and M-20 is updated to match.
