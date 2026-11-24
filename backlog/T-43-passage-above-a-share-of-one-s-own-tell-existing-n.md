---
type: Task
title: 'Passage above a share of one''s own: tell existing names from missing ones or not'
status: erledigt
points: 2
id: T-43
milestone: M-5
created: '2026-11-24'
---
# T-43 — Passage above a share of one's own: tell existing names from missing ones or not

Open in M-5 since the attack round on Samba (2026-11-20), a decision of the maintainer. Someone who holds a role deep below a folder they cannot read gets a share of their own; the path above it is a passage. Question: may the passage answer differently for a name that exists and one that does not (which tells that a folder of that name exists), or must both answer the same? Prepare the options with what each costs, the maintainer decides, then build and test it.

**Decided and built on 2026-11-24:** the maintainer chose option B (veto per user and share with Samba's own means;
A keep, C bind mounts in a privileged helper, D a module of smbd were the others). Built as in
[ADR-30](ADR-30-samba-share.md#settled-on-2026-11-24-the-passage-tells-no-guessed-names-t-43) and
[docs/samba.md](../docs/samba.md#limits); the collision with a readable name of the same name stays open for that name,
with a log entry.
