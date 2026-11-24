---
type: Task
title: 'Agents and widened readers: old versions, restore from trash, re-uploaded files'
status: offen
points: 8
id: T-40
milestone: M-30
created: '2026-11-24'
---
# T-40 — Agents and widened readers: old versions, restore from trash, re-uploaded files

Left open by the attack round T-38 (2026-11-24) because they need a design decision. Proposals by the master; the maintainer may change them before this is built.

- **Old versions (path 4):** after a page was restricted, content removed, and then opened or moved to wider readers, old versions show the removed content to the new readers. Proposal: the history shows a reader only versions from the time they could read the page; owner and administrators see all.
- **Restore from trash (path 5):** restoring to a parent that has more readers since the delete. Proposal: keep the readers at the time of the delete; an agent restores only to a place with the same or fewer readers; a person gets a warning with the readers line when the target is wider.
- **Download and upload again (path 11):** an agent uploads a file with the same sha256 as a file fewer people may read. Proposal: refuse for agents, with a hint. This catches only identical files; anything else is the mark of T-38.
- **Confirm:** the agent read context of T-38 is two hours per person (table agent_reads), because MCP is stateless.
- Test for the data part of migration 039 (a private rules page taken over from the tag is dropped), missing since B-6.
