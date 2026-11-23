---
type: Task
title: Archive for groups and spaces, and a Members preset
status: erledigt
id: T-34
milestone: M-30
created: '2026-11-23'
---
# T-34 — Archive for groups and spaces, and a Members preset

Today there is only the trash with 30 days. The team wiki keeps an archive without a limit, and its import needs a target for archived groups and spaces.

- **Archive** as a state of groups and spaces: hidden from the tree and from search by default, readable for those with rights, restorable, kept without a limit. Archive and change the owner only by the owner (or an administrator).
- **Preset Members:** manage members and rights, but no delete and no archive. The preset Manage keeps delete.
- The team wiki's moderator maps to Members.

**Built on 2026-11-23** (migration 037; [docs/rights.md](../docs/rights.md#archived-spaces), ADR-36 part 3). Decided in
the build:

- **Groups** had their archive since B-3; B-5 narrows who archives them to administrators. Nothing new for groups here
  but `db.ImportGroup`, which makes a group archived for an import.
- **Only spaces are archived**, not collections inside them: a space has one owner who answers for it, archiving and
  restoring stays a question of the whole space, and what lies inside one is moved to a space of its own first. A check of
  the database keeps the mark at the top. Spaces of the documents and of Files alike; never private notes or the folders
  of the workspace and of users.
- **Archived is read only for everyone** until it is restored, the owner and administrators included, so nobody changes
  what the archive keeps by mistake. It is one rule of the one check (`object_decide`): comment, create, write, delete
  and share are refused inside an archived space (deciding entry `archived`). So every door holds it without a check of
  its own: the API, MCP and its upload links (also one made before), Samba (read only), uploads in the browser (also one
  started before), share links (they lend nothing while nobody may share; restoring brings them back), moving, copying
  in, restoring from the trash. **Manage stays**, so who may read the archive can still be set; read stays as the entries
  say.
- **Who archives, restores and transfers:** the owner or an administrator in a browser session, never an agent, a
  service account, a key or an app password (as purge in B-3). Transferring the ownership now needs the browser session
  everywhere, not only for spaces.
- **Hidden** from the tree (`/api/state`, `/api/files`), the search (a filter on the collection at the top, so the counts
  leave it out too, no new index) and the MCP tools `context` and `search`, until asked for (`archive=1`, `archived`).
  An address still opens what lies inside, for whoever may read it.
- **The preset Members** is every action but delete. It manages members and rights and edits, but deletes and archives
  nothing. To keep it that way, **nobody opens up an action they do not hold** on the object (allowing it, or taking back
  a deny of it, also for the tenant and through `/access`); narrowing is always allowed; restoring the inheritance needs
  all seven actions there; and a link to comment or edit lends only the actions its maker still holds. Administrators are
  not bound, as ADR-36 says.
- **Import (T-28):** an export marks an archived space (`archived: true`), an import makes it archived again,
  `underdoc import --archived` archives what it makes, `db.ArchiveAsOperator` and `db.ImportGroup` (archived or not) run
  only without a request. The mapping moderator → Members stands in T-28; the import itself is T-28.
- **The interface** has no German table yet (T-32 brings it for every text); the words of the archive stand in one table
  (`TA` in demo.js) for it.
- Attack round in `internal/db/archive_test.go`, `internal/web/archive_test.go` and the browser tests of the archive;
  every guard is proven by a mutation.
