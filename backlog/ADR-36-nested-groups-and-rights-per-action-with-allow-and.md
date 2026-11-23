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
- A right granted to a group holds for its subgroups by default. The rights dialog shows a box "with subgroups", ticked and
  removable.
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

Confirmed by the maintainer on 2026-11-23.

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

### The built-in group Agents

Confirmed by the maintainer on 2026-11-23, sharpened with the gate.

- **Who is in it:** every request whose sign-in is not an interactive `session`, which means `oauth`, `apikey` and
  `app-password`. A missing or unknown value counts as an agent (fail closed). The value is taken only from the result the
  gate's Go client has checked, never raw from the header (ADR-35).
- **Service accounts** (`Remote-Kind: service`) are a case of their own. They get what is granted to them (ADR-35), not this
  group.
- **A deny on Agents is a ceiling, not an entry in the order above.** After the normal evaluation, an action denied for
  Agents on the object or above it is refused, even if the person is the owner or holds an allow through any other way.
  Test: owner + `oauth` + deny on Agents → refused.
- **What agents may by default:** everything their person may, except:
  - manage rights
  - share (links)
  - delete for good
  - read passwords (ADR-7)

  Undoable work stays allowed: read, create, write, comment and moving to the trash.
- **A space can be set to "no agents"**, which is a deny on read for Agents there.
- **Every person can limit their own agents**, for example to read only.

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
- **Built on 2026-11-23** ([docs/rights.md](../docs/rights.md), migration 030, T-26). Settled in the build:
  - The owner of something above holds every action there as an allow on that level, so a deny on the same level beats
    it like any other; only on their own object is the owner beyond a deny.
  - A deny on Agents holds on the object and everywhere above it, whatever breaks the inheritance, so a manager below a
    space with no agents cannot let agents in by breaking it. Agents can only be denied: an allow for Agents would give
    every agent what its person may not hold.
  - The way of signing in reaches the check through `account.User.AuthKind`: a session of the sign-in pages is a
    session, the gate's `Remote-Auth` ~~is read as `Remote-User` is until T-27 takes it from the checked result~~ comes from
    the result of the gate's Go client (ADR-32, 2026-11-23), and an
    empty or unknown value is an agent. An agent is no administrator for its request, takes no share link for its
    person and makes no app password for the share, since the share decides each person as a person.
  - Moving to another place needs delete on the object and on everything below it, as a deletion does, and create where
    it goes; it is refused where it would let the mover share or manage the object there (the attack of ADR-29,
    2026-11-19, under actions).
  - Breaking the inheritance copies, for each holder and action, the nearest entry from above. Where entries above
    overlap, deny on the one level now wins, so breaking never gives anyone more.
  - A group admin puts only groups inside theirs that they manage already; otherwise putting a group inside would make
    them its admin.
  - The search index keeps the owner and up to six levels of entries on read with their allowed and denied tokens, and
    whether no agent may find a record; each hit is checked once more in the database, so a deny holds while the index
    catches up. A record with more levels is found by its owner alone.
  - Samba: each account is decided on its own, so a deny on a folder holds on the share without a deny entry in an ACL;
    a deny on a single file of a shared library is refused, as breaking its inheritance is (T-1 proves no rights per
    file).
