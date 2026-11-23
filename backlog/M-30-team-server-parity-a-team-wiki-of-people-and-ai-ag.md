---
type: Milestone
title: 'Team server parity: a team wiki of people and AI agents moves in'
status: offen
id: M-30
tags: [ai, mcp, import]
blocked_by: [M-5]
created: '2026-11-23'
---
# M-30 — Team server parity: a team wiki of people and AI agents moves in

The maintainer runs a separate team wiki: Markdown in Git, written by people and AI agents over MCP, with about 70 tools.
It stays private and keeps running until UnderDoc can take its place, and then moves in by import. Its code (Go, MIT,
by the maintainer) serves as a template for this milestone. What is taken over is translated on the way.

Until then, both keep the same syntax and concepts: page kinds, embedding, `id:`, part marks, and the C22 pack underdoc.

**Done when this is true:**

- **Two audiences** as in ADR-33: agent documentation and reference on one side, human documentation on the other, with
  visible, checkable AI changes.
- **MCP with the same behaviour** (T-21):
  - write a page or a part against a base version, so two writers do not overwrite each other
  - check at write time: errors block, warnings come back with a fix hint
  - topic tree; search with synonyms
  - move with a lasting redirect; history and diff per page
  - one-time upload links for large files; a PDF page as an image
  - hints for a file nobody links to and for a link to nowhere
  - tools for collections and groups
  - the agent skill is delivered with a version and updates itself
- **Data pages:** a CSV file shows as a table (ADR-27), and an export replaces the file in place.
- **Automatic sections**, computed on display and never stored:
  - the cases of a customer and of a person
  - "linked from here"
  - pages of a folder that no overview links to yet
  - a page per tag
- **Sign-in through TinySesam** (ADR-32):
  - service accounts are told apart from people and fail closed
  - OAuth for coding agents
  - group admins invite colleagues, lock them out and send a new link through the gate's account API
- **Rights** (M-5):
  - roles for persons and groups, passed down to subgroups
  - exceptions per folder, for example a feedback page everyone may write to
  - a test channel and a stable channel
- **Import:** a Git Markdown wiki with author and date per version. Old paths become redirects (M-17).
- **Connectors:** access to outside systems, shared or per person, stored encrypted. A private connector plugs in at an
  extension point (M-4).
- **Navigation:**
  - tree and breadcrumbs
  - a folder view with its overview
  - "used in" for pages and files
  - a table of contents that marks where the reader is
- **Operations:**
  - an export of everything to Git and Markdown as a way out
  - a backup that has been restored once

  These replace what "Git is the truth" gave the team wiki.
