---
type: Task
title: 'Folder mode "reviewed": changes by agents to human documentation wait as a proposal'
status: offen
points: 5
id: T-35
milestone: M-20
created: '2026-11-23'
---
# T-35 — Folder mode "reviewed": changes by agents to human documentation wait as a proposal

Follow-up of T-20 (ADR-33). Today a change by an agent is live at once and carries the hint "written by AI, not checked"
until a person checks it or takes it back. ADR-33 also decides that a folder can be switched to **reviewed**:

- In a reviewed folder, a change by an agent to human documentation (how-to, explanation, tutorial, or no kind) does not
  change the page. It waits as a proposal of its block, with the diff of the check of T-20.
- Readers see the checked state; whoever may change the page sees the waiting proposal and accepts it ("Checked", which
  then writes it) or drops it ("Take back", which then leaves the page as it is).
- The agent side (reference, agent notes) and structure pages never wait.
- The switch is a setting of a folder or collection, inherited below, set by whoever manages it. Agents may not set it.
- Builds on the open changes of migration 034 (`ai_changes`, `block_versions.agent`) and the check and taking back of
  `db.CheckChange` and `db.TakeBack`; fits into the review of blocks of M-20 (ADR-22: proposed changes are reviewed
  block by block).
- Attack round: an agent writes into a reviewed folder through every door (API, MCP of T-21, moving a block in), and
  its text reaches no reader until a person accepted it.
