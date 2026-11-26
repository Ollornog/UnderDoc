---
type: Bug
title: 'Review of T-15 and T-43: owner hand-over race, rights lock without timeout, veto list per session,
  veto by spelling'
status: offen
points: 8
id: B-10
milestone: M-5
created: '2026-11-26'
---
# B-10 — Review of T-15 and T-43: owner hand-over race, rights lock without timeout, veto list per session, veto by spelling

Findings from the team server review of T-15 and T-43 (2026-11-24), all medium, nothing high:

- Owner hand-over checks the owner only before taking the lock. Two concurrent hand-overs can leave the wrong owner.
- The rights mutex has no context or timeout.
- The %U veto list may be wrong when several sessions share one connection.
- Can a veto be bypassed by a different spelling (case, Unicode form) or an 8.3 short name?

Each finding needs a test that fails before the fix (mutation).
