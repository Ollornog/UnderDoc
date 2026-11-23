---
type: Task
title: Import and export of a Git Markdown wiki, with a round trip that loses nothing
status: offen
id: T-28
milestone: M-30
created: '2026-11-23'
---
# T-28 — Import and export of a Git Markdown wiki, with a round trip that loses nothing

Import a Git Markdown wiki with author and date per version, folders, overview pages, front matter, ids, part marks, embeddings, datei: links with page anchors, images at their step, and the move file as redirects. Export everything back to Git Markdown as the way out (M-30 operations). A round-trip test on a synthetic corpus that imitates a real team wiki (raw umlaut paths, angle-bracket placeholders as text, pipes in code spans inside tables, indented images in numbered lists, quoted descriptions, part marks inside tables and lists, CRLF in old files, no BOM) must give an empty diff. Automatic sections never appear in the export. No real company content in the repo or on test systems.

Rights and accounts from the team wiki's review (2026-11-23):

- **Accounts** are created ahead with `underdoc account add` (T-33), bound by Remote-Id. Profiles without a Remote-Id are not bound, names never stand in for one.
- **Mapping of rights:** owner → owner, moderator → Members (T-34), write → Edit, read → Read, everyone signed in → tenant, a group right with sub-teams → group with subgroups. A leader becomes group admin and member.
- **Path rules:** "only these may read" breaks inheritance; "read only" becomes a deny on create, write and delete per person.
- **Archived groups and spaces** go to the archive state (T-34); a lock and a delegated admin go to T-33.
- The export does not carry rights and denies yet, so the import needs its own way for them, with a mapping of IDs.
- **Part addresses:** the team wiki uses `#tN`; UnderDoc needs `#bN.tN`.
- **Front matter:** two axes there (`type` and `art`), one here (`kind`); the mapping comes from T-20.
