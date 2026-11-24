---
type: Task
title: 'Inbox list "AI changes to check"'
status: offen
points: 3
id: T-36
milestone: M-16
created: '2026-11-23'
---
# T-36 — Inbox list "AI changes to check"

Follow-up of T-20 (ADR-33). The hints of changes by agents to human documentation are found today on the page and with
the search filter "AI changes to check". ADR-33 also wants them in the inbox of M-16:

- An inbox list "AI changes to check" shows, for each page the person may change, the blocks an agent changed and nobody
  checked: page, block, whom the agent works for, and when, newest first.
- Each entry opens the diff of the check of T-20; there is no "check all", so every check is seen.
- The list is the person's own: it names only pages the person may read and change, and an agent never sees it as its
  own work list to clear.
- Hints never expire on their own (ADR-33). If the list grows, it says how many there are, not less.
- Builds on `db.PagesToCheck` and `db.AIChanges`.
