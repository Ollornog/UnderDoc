---
id: ADR-15
type: Decision
title: A workspace of typed documents, with three views
status: erledigt
tags: [architecture, ui]
created: 2026-11-06
---

## Context

Tabs for documents and data would split things that belong together: a table could not sit next to the page that explains
it. There are really three different questions: how do I want to arrange my work, what lies on the disk or share, and what
has no place in any folder at all?

## Options

- **Tabs by kind (documents, data, databases):** easy to explain, but a table cannot join a page.
- **One tree that mirrors the disk:** simple, but every arrangement is tied to where files lie.
- **Three views on one set of objects:** arrangement and storage stay independent, at the price of explaining two trees.

## Decision

The third option.

- **Workspace:** collections on top, documents below, arranged as the user wants. This is the navigation that exists today.
- **Any document can have children.** A page has subpages, and a table can have a page below it. A folder is just a document
  without content.
- **Document types:** page, table, Excel file, image collection, database view, database, and later site (M-21). Each type
  is a module (ADR-14).
- **Files:** a tab with the folder structure on the disk or share, where images and the like are stored. It follows the disk
  closely but not strictly: temp and lock files stay hidden, for example.
- **Resources:** a tab for what has no place in a folder, such as values, snippets, connections and external sources. It also
  lists every database and view, wherever they are placed.
- **One object, several views:** placing a file or database in the workspace adds a place, not a copy (ADR-8). Moving it in
  the workspace does not move it on disk, and the reverse holds too.
- **On disk,** a page with children is `page.md` next to a folder `page/`. Folder sources and exports use the same rule.

## Consequences

- ADR-13 stays valid for identity, roles, content and the share. Its folder tree becomes this workspace tree.
- Cycles are refused, and moving a document moves its children.
- Which tree decides inherited roles is not settled, because a share can only enforce what the disk shows (T-9).
- The facts panel shows both the place in the workspace and the path on disk, so two trees stay understandable.
- How a type behaves (rendering, facts, file link) lives in its module, so a new type changes nothing else.
- T-5 tries the three views in mockups before they are built.
