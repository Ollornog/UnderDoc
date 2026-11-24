---
type: Task
title: Follow-ups of the permission work
status: erledigt
points: 5
id: T-15
milestone: M-5
created: '2026-11-22'
---
# T-15 — Follow-ups of the permission work

Points left over from the permission work (ADR-29):

- Test the toast text shown when an action is refused.
- Concurrency of permission changes on deep levels.
- The default when ownership is transferred.
- Attachments inside links.
- Internal links to a frozen page.
- Person and group permissions in export and import (T-14: an export carries no permissions).
- Old organisation administrators from before ADR-29 must not survive on a production system.
- ADR-7 (passwords outside Markdown) in the light of the permission model.

**Closed on 2026-11-24, point by point:**

- Toast of a refused action: tested since T-26 (`TestARefusalSaysWhy`, browser: a refused move and a refused deletion
  each say why).
- Concurrency on deep levels: built now. Changes of rights of a tenant go one after the other (a lock of the database,
  and a lock of the process before they take a connection), breaking the inheritance copies what holds above under that
  lock, and every change decides again under it whether its actor manages the object. Before, a deny given above while
  a page deep down broke its inheritance was lost to a copy read before it (`TestRightsChangesOnDeepLevelsGoOneAfterTheOther`,
  [docs/rights.md](../docs/rights.md#known-limits)).
- The default on a transfer of ownership: settled with ADR-29 and T-34, the former owner keeps the preset edit, which can
  be taken back; only the owner or an administrator in a browser session (`TestTransferringOwnership`).
- Attachments inside links: built 2026-11-20, the attachments a frozen page shows come through the link
  (`TestAFrozenPageShowsItsAttachments`).
- Internal links to a frozen page: built the same day, a link into the workspace becomes plain text marked "Not part
  of this link", a link to a heading of the page stays (same test).
- Person and group permissions in export and import: built now, as `rights` beside `access`, persons by login and
  groups by name, also the tenant beyond its preset and the denies on Agents, for documents and files; an entry whose
  holder is not here is left out and named (`TestEntriesOfPersonsAndGroupsGoOutAndComeBack`, `TestBackupOfTheFiles`,
  [docs/export.md](../docs/export.md)).
- Old organisation administrators from before ADR-29: UnderDoc made administrators of the time tracking without its mark
  (`ud-admin-`) only between #81 and #87 on 2026-11-19, a few hours; there is no production system, and the one test
  system holds no membership of the time tracking at all (checked read only on 2026-11-24). Nothing to carry along.
- ADR-7 in the light of the permission model: revealing goes through `secret_may`, which asks the one decision of ADR-36
  (allow and deny per action, Agents never); a deny of read above the page takes the password away
  (`TestADenyHidesASecret`, new), and a restriction narrows further (`TestWhoRevealsASecret`).

Mutations of the new parts: 14, 13 red and one equivalent (the check of the action name before the entry is written,
which the list of actions makes again).
