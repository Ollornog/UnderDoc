---
id: T-7
type: Task
title: Decide how reviews work, with which forge
status: offen
milestone: M-20
tags: [review, git]
created: 2026-11-06
---

- **A self-hosted forge next to UnderDoc:** Forgejo (GPL-3.0) or Gitea (MIT, open core) runs as a separate, optional service,
  and UnderDoc talks to its API. Review and issues stay there, and nobody needs a GitHub account. It is one more service to
  run. This is the starting recommendation, with Forgejo.
- **GitHub only:** nothing extra to run, but reviewers need a GitHub account and private data leaves the server.
- **Review inside UnderDoc:** works for every file type and user, but rebuilds part of a forge.

The forge is not compiled into UnderDoc, which keeps the license and updates separate. Settle whether the API covers everything
M-20 needs (branches, pull requests, review, issues), how sign-in is shared, how Git-backed folders fit ADR-13 and ADR-15 (Git
stays their source, like a folder source) and how they fit real-time editing (M-10). Excel files show no useful diff in any option.

**Done when:** the choice is recorded in a decision, and M-20 is updated to match.
