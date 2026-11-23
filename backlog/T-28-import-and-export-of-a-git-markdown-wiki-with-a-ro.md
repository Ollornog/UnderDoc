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
