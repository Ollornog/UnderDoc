---
type: Task
title: Rights per action with allow and deny, nested groups and the group Agents
status: offen
id: T-26
milestone: M-30
created: '2026-11-23'
---
# T-26 — Rights per action with allow and deny, nested groups and the group Agents

Build ADR-36 on top of ADR-29: nested local groups (roles of the gate can be members, cycles refused, a box "with subgroups" ticked by default); the actions read, comment, create, write, delete, share and manage, each inherit, allow or deny per person or group; the nearest entry wins and deny beats allow on the same level; owner, administrators and private objects as ADR-36 says; the built-in group Agents (every sign-in that is not a session, fail closed) as a ceiling after the evaluation; existing grants migrate to allow entries of their preset. Deny holds in the tree, search (reader tokens), embedding, API, MCP and exports. The access check names the deciding entry. An attack round before release.
