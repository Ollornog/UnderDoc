---
id: M-20
type: Milestone
title: Reviewed documents and Git
status: offen
blocked_by: [M-5, M-8]
tags: [review, git]
created: 2026-11-06
---

Some data can simply be changed. Other data, such as code, needs someone to review a change and confirm it. T-7 decides
how this is built and with which forge.

**Done when this is true:**

- A part of the workspace is either free (a save becomes the current version) or reviewed (a change waits for approval).
- Readers of a reviewed part see the approved state, and a proposed change shows as a diff.
- A Git repository can be a source, and code files render with syntax highlighting.
- Editing a Git-backed document creates a branch and a pull request.
- Review, comments and issues are visible from the document and link to where they live.
- UnderDoc also works without a forge. The forge is an optional module.
- Access tokens come from the environment and are limited to the configured repositories.
