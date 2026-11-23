---
type: Decision
title: Nested groups, and rights per action with allow and deny
status: erledigt
id: ADR-36
tags: [permissions, groups]
created: '2026-11-23'
---
# ADR-36 — Nested groups, and rights per action with allow and deny

## Context

ADR-29 kept groups flat and knew only allowing. To narrow a right, an object breaks its inheritance. A team wiki behind
the same gate (ADR-35) shows that a tree of groups with admins per branch fits a company well. On 2026-11-23 the
maintainer decided:

- Local groups nest, and nesting passes rights down.
- Rights can be denied explicitly, not only granted.
- Each right is a single action, for example create, read, write or delete, and each can be *inherit*, *allow* or *deny*.

This replaces the parts of ADR-29 that say otherwise. What ADR-29 settles about the trees, private objects, links, Samba
and administrators still holds.

## Options

- **Keep ADR-29:** simple to read, but a company has to break inheritance often, and groups have to be kept flat by hand.
- **Rights in the gate:** rejected in ADR-35, because the gate does not know the objects of an app.
- **Actions with inherit, allow and deny in the app, and nested local groups.** This is the choice of the maintainer.

## Decision

### Groups

- A **local group** contains persons, other local groups and roles taken over from the gate. A role from the gate can be a
  member of a local group, but it does not nest itself (ADR-35).
- Membership is passed down: whoever is in a group inside group G is also in G. A cycle is refused when it is made.
- A local group has admins. They manage the group and the groups inside it. Administrators of the tenant manage all groups.
- The interface says "group" for both kinds and marks where each comes from (ADR-35).

### Actions

| Action | Allows |
|---|---|
| **read** | see and open, read comments, embed what it reads |
| **comment** | add comments and replies |
| **create** | make children: pages, folders, files, uploads |
| **write** | change content, title and tags, reorder, see versions |
| **delete** | move to the trash and restore |
| **share** | make and revoke share links |
| **manage** | change the rights of the object, break and restore inheritance |

- On an object, each action is set per person or group to **inherit** (the default: take it from above), **allow** or
  **deny**.
- **The roles of ADR-29 stay, as presets**: read, comment, edit, manage, owner. The rights dialog shows the presets first and
  the single actions on request. A preset that has been changed shows as "custom".
- Moving needs delete where the object is and create where it goes.

### Which entry wins

This is the proposal of UnderDoc; the maintainer can correct it.

1. For each action, UnderDoc looks for the **nearest object** that has an explicit allow or deny for the person or for any
   group they belong to. It starts at the object itself and walks up.
2. On that object, **deny beats allow.**
3. If no object up to the top has an entry, the action is denied.
4. **Breaking the inheritance** still exists. Nothing comes from above, as in ADR-29.

This is the order most people know from file servers: what is set closer wins, and on the same level deny wins.

**Limits of deny:**

- **The owner** keeps every action on their own object. A deny for a group they belong to does not lock them out.
- **Administrators** manage rights, not content (ADR-29). Deny cannot stop them from managing rights; their access to
  content follows the same rules as everyone else's.
- **Private stays private** (ADR-29). No right can be granted or denied there.

### Everywhere the same

A deny on read holds through every door: the tree, search, embedding, the API, MCP (ADR-34) and exports. An embedding of
something denied shows "not to be found" (ADR-18).

The access check in the tab Rights ("what may X do", "who sees Y") shows the action, the deciding entry and where it
comes from.

## Consequences

- The rights tables change from one role per entry to an action with a value. Existing grants become allow entries of
  their preset. Broken inheritance stays broken.
- The search filter (reader tokens, ADR-24) has to carry deny: a document is found only when read is allowed by the
  nearest entry.
- Samba: a deny is passed on as a deny entry of the share's ACL only as far as T-1 proves it survives. Otherwise the
  interface shows that Samba cannot express it, as ADR-29 does for rights per file.
- The access check and the log name the deciding entry, so that a deny is never a riddle.
- An attack round looks at deny before release: inheritance, nested groups, cycles, the owner, embedding, search.
