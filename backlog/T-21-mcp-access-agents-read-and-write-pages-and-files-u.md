---
type: Task
title: 'MCP access: agents read and write pages and files under the user''s permissions'
status: offen
id: T-21
milestone: M-13
created: '2026-11-22'
---
# T-21 — MCP access: agents read and write pages and files under the user's permissions

M-13 has agents read over MCP. A team wiki run by people and AI agents together shows that they also need to write, and what that takes:

- Read, search, list a section, show the history and an older version.
- Write a whole page, or replace a part with a base version, so two writers do not overwrite each other.
- Move a page and leave a redirect; delete means archive.
- Files: read, search inside PDFs, attach, restore.
- The server sends instructions for agents with the conventions of the instance (page kinds, where things go).
- Every call runs under the permissions of the signed-in user (sign-in through the gate, ADR-32) and is logged (ADR-11).
- Sign-in for agents: OAuth or an API key from the gate, never a password in a command line.

Independent of a model on the server: this works without the AI drawer.
