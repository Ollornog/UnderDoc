# Rights

Who sees and changes what, for documents and files alike ([ADR-29](../backlog/ADR-29-roles-on-groups-and-persons.md),
[ADR-36](../backlog/ADR-36-nested-groups-and-rights-per-action-with-allow-and.md)). It replaces the one level for all
others of ADR-20; the owner, the private collections and the rules for comments stay. Since ADR-36 a right is a single
action, set to inherit, allow or deny, and local groups nest.

## Actions

| Action | Allows |
|---|---|
| **read** | see and open, read comments, embed what it reads |
| **comment** | add comments and replies, on the whole page or on one block |
| **create** | make children: pages, folders, files, uploads, copies into it |
| **write** | change the content, title and tags, reorder, see the versions |
| **delete** | move to the trash and restore |
| **share** | make and revoke share links |
| **manage** | change the rights of the object, break and restore the inheritance |

On an object each action is set per person, group or the whole tenant to **inherit** (no entry, the default: take it
from above), **allow** or **deny**. **Moving** to another place needs delete where the object is and create where it
goes, and delete on everything below it as a deletion would; it is refused where it would give the mover there a right
to share or manage it that they do not hold here (they would own what is above it). Reordering within the parent needs
write.

### Presets

The roles of ADR-29 stay, as presets of actions. The rights dialog shows the presets first and the single actions on
request; entries that are no preset show as **custom**.

| Preset | Actions |
|---|---|
| **Read** | read |
| **Comment** | read, comment |
| **Edit** | read, comment, create, write, delete |
| **Manage** | all seven |
| **Owner** | all seven, and transferring the ownership; nobody takes it away |

**The owner** is one person per object, whoever made it. Entries go to

- **a person:** an account of the tenant;
- **a group:** a **local group**, or a **group taken over** from OIDC (the groups claim) or the gate (`Remote-Groups`):
  each name a sign-in brings is a group, and its members are the accounts whose last sign-in named it. A group taken over
  can only be read here (below);
- **the tenant:** every account of the tenant;
- **Agents:** the built-in group of agents, only ever denied (below).

## Which entry wins

1. For each action UnderDoc looks for the **nearest object** that has an entry, allow or deny, for the account, for one
   of its groups or for the tenant. It starts at the object itself and walks up while the object inherits.
2. On that object **deny beats allow.**
3. If no object up to the top has an entry, the action is **denied**.
4. **Breaking the inheritance** stays: nothing comes from above it.

Limits of a deny:

- **The owner** keeps every action on their own object; a deny for a group they belong to does not lock them out. The
  owner of something above holds every action there as an allow on that level, which a deny on the same level beats
  like any other, so it reaches the objects below only while nothing nearer decides.
- **Administrators** manage rights whatever a deny says, and read content only where the entries let them (below).
- **Private stays private**: nothing is allowed or denied there.
- **A share link** to comment or edit that an account opened counts as an allow of its preset on the link's object.

A grant to a group holds for the groups inside it by default (**with subgroups**, a box in the rights dialog); without
it, only for the persons and groups taken over that are directly in the group.

## Groups

- A **local group** contains persons, other local groups and groups taken over from the sign-in. A group taken over
  (a role of the gate) can be inside a local group, but contains nothing itself (ADR-35). Membership is passed down:
  whoever is in a group inside G is also in G. A group can never be inside itself, also not through others: the
  database refuses it under a lock, so two changes at once cannot close a cycle.
- A local group has **group admins**. They manage the group and every group inside it: members, groups inside it,
  admins, name, deleting it, and making a new group inside it. Administrators of the tenant manage all groups.
- The interface says "group" for both kinds and marks where each comes from.

## The group Agents

- **Who is in it:** every request whose sign-in is not an interactive session. A session in the browser (passkey,
  password, single sign-on) is one; through the gate the way comes from `Remote-Auth` (`session`, `oauth`, `apikey`,
  `app-password`), and a missing or unknown value counts as an agent (fail closed). Until T-27 takes it from the result
  the gate's Go client checked, UnderDoc reads `Remote-Auth` as it reads `Remote-User`: trusted because the proxy of the
  gate sets it on every request. In the code it is `account.User.AuthKind`, and `account.User.Agent()` decides.
- **Service accounts** (`Remote-Kind: service`) are a case of their own: they get what is granted to them, not this
  group.
- **A deny on Agents is a ceiling** after the evaluation above: an action denied for Agents on the object or anywhere
  above it is refused, for the owner too, and breaking the inheritance below it does not lift it.
- **Agents never** manage rights or groups, share (links), delete for good (empty the trash) or read passwords
  (ADR-7). Read, create, write, comment and moving to the trash stay as their person may.
- **No agents** on a space is a deny on read for Agents there, set like any entry (kind `agents`).
- **Every person limits their own agents** (`PUT /api/me/agents`), for example to read only; only the person in a
  session changes it, never an agent.

## Inheritance

An object holds what is decided on it and on everything above it, up to the top or to an object that **breaks the
inheritance**. Breaking copies onto the object, for each holder and action, the nearest entry from above, allow or
deny, so nobody gains anything; where entries above overlap (an allow for someone nearer, a deny for a group of theirs
farther up) deny on the one level now wins, so check the access check after breaking. Then take back what shall not
hold. Restoring the inheritance takes from above again and keeps the object's own entries. Denies on Agents are not
copied: they hold as a ceiling anyway.

- **A new collection at the top belongs to its owner alone** until they share it (2026-11-19; before, it granted edit to
  the whole tenant): no role for anyone else, not even the tenant. It is not private, so it is shared like any other. The
  dialog that makes it has two steps, the name and then **Share with** (Enter in the name makes it at once, shared with
  nobody): everyone in the workspace, groups and persons, each with a role, by default nobody; it grants through `PUT /api/objects/<id>/grants`, as the rights dialog does. The same holds for a new
  collection of Files and for a copy made at the top; a collection inside another inherits. Collections made before keep
  what they granted.
- A move keeps the object's own entries and its owner; what it inherits changes with its new place. **Moving to another
  place needs delete** on the object and everything below it and create on where it goes, and never makes the mover
  someone who may share or manage it (until ADR-36: manage on the object and edit on where it goes); within its parent, write reorders it. The
  workspace opens no gap for a move one may not make.
- **Private stays private:** My notes, the folder of each user in Files and everything below them belong to their user
  alone. Nothing can be granted there, and administrators see nothing of it. Only its owner moves an object into
  something private, and only when everything below it is theirs too; otherwise the move is refused with a message,
  and nothing new comes in anywhere below while that is checked. Its
  grants and links end with the move, and the log keeps each for the owner.
- **Deleting takes nothing one may not delete:** deleting or purging needs delete on the object and everything below it. A
  page below that breaks the inheritance and shuts the deleter out refuses the deletion with a message; its owner moves
  it out first.
- **Versions are for write:** the history of a page or a file holds what was taken out, so whoever only reads gets the
  current state, not the versions.
- **Embedding bypasses nothing:** an embedded page or block shows "not to be found" to whoever may not read it, a deny
  included; an
  embedded table of a spreadsheet the same, without the name of its file ([tables.md](tables.md#tables-in-pages)).
- **Passwords in the text** ([secrets.md](secrets.md), ADR-7) are revealed by whoever reads their page, or narrower by
  whom its managers name; a role lent by a share link reveals none.

## On the Samba share

In a library shared over Samba ([samba.md](samba.md), ADR-30) the actions of each account become ACLs per folder: read
where it may read, write where it may read, create, write and delete. Each account is decided on its own, so a deny on a
folder holds on the share as in the web. Samba knows rights per folder only (T-1): an allow on a single file holds in the
web only, and the rights dialog says so; a deny on a single file, or breaking its inheritance, is refused there (`409`),
since the share would show what the web hides. A folder can do both. A folder in which someone may read without reading
the folder above is a share of its own for them.

## Administrators

Administrators of the tenant ([sign-in.md](sign-in.md)) **manage rights, not content.** They read and change the entries
of every object that is not private, whatever a deny says, break and restore its inheritance and transfer its ownership,
and they make local groups and manage them. They read the content of an object only where the entries let them, like
everyone else. Their agents manage nothing.

**What someone left behind:** UnderDoc deactivates no account; the gate or the identity provider ends the sign-in. An
administrator asks "what may they do" for that account in the tab Rights, which lists what it owns by title, and
transfers each in place to another account, which the form leaves unchosen until the
administrator chooses. Transferring shows the administrator nothing of the content, unless they
give it to themselves; the log keeps every transfer (`owner`).

## In the workspace

- **Rights and sharing** in the settings of a page and in the overview of a collection opens the rights dialog: the
  owner and the transfer of ownership, whether it takes the roles from above, the roles granted here (to everyone, a group
  or a person, changed or taken back in place), what it inherits, and its share links with a form for a new one. Whoever
  only reads sees the same, without the means to change it. The entries of each holder show as a preset; **Actions**
  opens the seven actions, each inherit, allow or deny, with the box **with subgroups** for a group. **No agents**
  denies read to Agents there.
- **The facts panel** of a page names its owner and who sees it with which role ("Visible to").
- **The tab Rights** in nav1: **Groups** (administrators make local groups, group admins make groups inside theirs; both
  take members, groups and admins in and out; groups from the sign-in are marked and read only; everyone else sees the
  groups they belong to and those they manage), **My agents** (what one's own agents may do), **Users** (administrators see every
  account with how it signs in and its groups, make a local account an administrator or take that back, and ask what an
  account may do, with what it owns transferred in place), **Share links** (administrators all of them, everyone else theirs and those on what they manage, each
  revoked in place), and the **Access check** ("what may someone do": where an account holds entries of its own, through
  a group or as everyone; "who sees something": the entries of an object, and for an account, also as an agent, each
  action with ✅ or ❌, the deciding entry and the object it is on).
- **The administration in the profile** shows Users and groups the same way (ADR-16).
- **Taking back an administrator** works for local accounts, by another administrator, so a tenant always keeps one;
  the database decides it under a lock, and a local administrator must still be one when it happens;
  accounts of the gate or of OIDC administer through their group at the sign-in. Whoever stops administering the tenant
  stops administering its organization in the time tracking where UnderDoc made them do so
  ([time-tracking.md](time-tracking.md)).

```text
GET  /api/rights/check?object=<id>&account=<id>[&agent=1]   each action of an account (or its agent) on an object, with
                                                   the deciding entry: via, object, kind, who, value
GET  /api/rights/held[?account=<id>]               where an account holds entries of its own
GET  /api/me/agents                                what one's own agents may do: {"actions": [...], "possible": [...]}
PUT  /api/me/agents                                {"actions": ["read"]}   in a session only
GET  /api/users                                    administrators: every account with its source and administration
PUT  /api/users/<id>/admin                         {"admin": false}   administrators, local accounts, not oneself
```

## The log

Access and changes are logged ([ADR-29](../backlog/ADR-29-roles-on-groups-and-persons.md), migration 024, table
`audit_log`): opening an object and changing it (each change of the API that went through, an upload, making something)
once per account, object and hour; roles, the inheritance, owners, groups, members, administrators and links each time,
with what was done; a link opened, used, a wrong password of a link and an upload through a link, with an empty actor for
someone without an account; grants taken back and links revoked by a move into something private, visible to its owner;
an action a deny refused (`refused`), with the deciding entry, once per account, object and hour; a change of what
someone lets their agents do (`agents`); and each time a password of
the text is made, revealed, copied, changed, restored, restricted, moved or deleted ([secrets.md](secrets.md)), never its
value. Nothing that lies in something private is logged. An entry keeps the plain IDs, so it outlives
what it names.

Administrators read the whole log in the tab Rights (and under Audit log in the administration of the profile), everyone
else what they did and what happened to what they manage. `GET /api/log[?object=<id>&actor=<id>&before=<entry>&limit=<n>]`
answers the newest entries first.

**Retention:** each tenant keeps an entry **365 days** (with one tenant: the instance). Administrators set 30 to 3650 days
in the section Log of the tab Rights; the change is logged (`log kept`, with the days before and after). The job that
empties the trash deletes older entries of every tenant at start and then every hour (table `log_settings`, migration
026).

```text
GET  /api/log/keep                   {"days": 365, "min": 30, "max": 3650}
PUT  /api/log/keep                   {"days": 730}   administrators
```

## One check

One function in the database, `object_decide`, decides an action of an account on an object (with `object_may` for the
account of the request), and every query that lists, finds or opens objects asks it: the tree, search, embeddings,
references, the trash, exports and the API. Whether the request is one of an agent reaches the database with each
connection the request takes (`underdoc.agent`, set from `account.User.Agent()`). `object_role` names the preset an
account holds for what the interface shows; it decides nothing.

The search index keeps who may read a record as levels (`object_access`, search source version 6): the owner, who always
finds it, then, from the record up, each object with entries on read and the tokens allowed and denied there (`all` for
the tenant, `account:<id>`, `group:<id>` for a group with its subgroups, `direct:<id>` for its direct members), and
whether no agent may find it. Meilisearch and OpenSearch filter by the same rule: the first level that names a token of
the reader decides, deny before allow. Levels that can never decide are pruned; a record with more than six levels left
is found by its owner alone. Each hit is checked once more against `object_may` before it shows, so a deny holds at once
even while the index catches up. A change of entries, of the inheritance or of the owner queues the object and everything
below it. The path of a record names only the levels above that every one of its readers sees, judged on the safe side
(a level that denies someone is always "…").

The tables `groups`, `group_members`, `group_groups`, `group_admins`, `agent_limits`, `grants`, `share_links`,
`share_link_users`, `share_link_passes`, `audit_log` and `log_settings` carry their tenant under row-level security like
all others ([tenants.md](tenants.md)). A row of `grants` is one action of one holder (`kind`, `who`) with `value` allow or
deny and `subgroups`. The column `objects.inherit` says whether an object takes anything from above.

## API

```text
GET    /api/objects/<id>/rights              owner, own entries, inherited entries, denies on Agents above, the viewer's role
                                             and actions; each holder with its "role" (preset or "custom"), "actions" and
                                             "subgroups"
PUT    /api/objects/<id>/grants              {"kind": "person|group|tenant", "who": "<id>", "role": "read|comment|edit|manage|"}
PUT    /api/objects/<id>/grants              {"kind": "person|group|tenant|agents", "who": "<id>",
                                              "actions": {"read": "allow", "write": "deny"}, "subgroups": true}
PUT    /api/objects/<id>/inherit             {"inherit": false}   break; true restores
PUT    /api/objects/<id>/owner               {"to": "<account id>"}
POST   /api/objects/<id>/access              {"access": "edit|comment|read|"}   the level for the whole tenant (ADR-20)
GET    /api/people                           accounts and groups to grant roles to
GET    /api/groups                           administrators: all; everyone else: their own and those they manage
GET    /api/groups/<id>                      the group, its persons, groups inside it, groups it is in, its admins
POST   /api/groups                           {"name", "description", "parent"}   administrators; group admins inside theirs
PATCH  /api/groups/<id>                      {"name", "description"}   administrators and group admins, local groups
DELETE /api/groups/<id>                      administrators and group admins, local groups; their entries go with them
GET    /api/groups/<id>/members              administrators, members and group admins
PUT    /api/groups/<id>/members/<account>    {"member": true}   administrators and group admins, local groups
PUT    /api/groups/<id>/groups/<group>       {"member": true}   a group inside a local group; a cycle answers 409
PUT    /api/groups/<id>/admins/<account>     {"admin": true}    administrators and group admins
```

An empty role, or no action, takes every entry of that holder back; an action left out of "actions" inherits. A preset
writes allow entries of its actions and nothing else. The level of ADR-20 (`/access`) stays for the settings of the workspace: a level narrower
than the one above breaks the inheritance and grants the tenant that level; an empty one takes the tenant's grant back and
restores the inheritance. At the top, where nothing is above, an empty one leaves the collection to its owner and who
holds a role of their own ("Nobody" in its overview, 2026-11-19; before, it meant edit).

## Share links

Whoever manages an object makes links to it ([ADR-29](../backlog/ADR-29-roles-on-groups-and-persons.md)), never on
something private. The address `/s/<token>` carries 32 random bytes; the database keeps only their SHA-256, so the address
is shown once, when the link is made. A link has an optional expiry and an optional password (Argon2id, with the limit
against guessing of the sign-in; a browser that gave it gets a cookie for that link alone, for a day). A link that expired,
was revoked, lies on something in the trash, or whose maker no longer manages its object answers like one that never was,
and lends nobody a role. The log names `/s/…` without the token, as
it does for invitations.

| Mode | For | What it does |
|---|---|---|
| **read** | anyone with the link | a page **frozen**; a file, or a folder of the files with what is below it, as it is now, and only what the maker of the link sees now, checked for each folder and file |
| **upload** | anyone with the link | into a folder of the files: each file arrives under a name nothing there has when it is made, as a file of whoever made the link, never as a version of what is there; the visitor sees nothing of the folder |
| **comment**, **edit** | accounts of the tenant | whoever opens the link while signed in holds that role on the object and below it, as long as the link lives; without an account, the link asks to sign in |

- **Frozen:** a page is rendered when the link is made, as its maker sees it then, and the link shows exactly that until a
  manager renews it. Every page it embeds, directly or through what it embeds, is listed (`/shareables`) and shows only
  if it was released; the rest says "Not shared". A collection or folder of the documents gets no link to read.
- **Listed and revoked:** `/api/links` lists the links with who made them, when, until when and how many accounts opened
  them; administrators see every link, everyone else the links they made and those on what they manage. Whoever made a
  link, whoever manages its object and administrators revoke it.
- **Attachments and links of a frozen page** (2026-11-20): the images pasted into the page, and into the pages it
  released, come through the address of the link, `/s/<token>/p/<page>/attachments/<id>`: only those the frozen page
  shows, and only while the maker of the link sees their page. A link of the page into the workspace (`/o/…` and every
  other address of the application) becomes plain text, marked "Not part of this link", since whoever holds the link has
  no way in there; a link to a heading of the page itself stays. A file of Files shown in the page is not part of a
  link.

```text
GET    /api/objects/<id>/shareables          the pages a page embeds, to release one by one
POST   /api/objects/<id>/links               {"mode", "expires": "2026-12-31", "password", "released": ["<id>"]}  → the link with its address
GET    /api/links[?object=<id>]              the links the viewer may see
POST   /api/links/<id>/renew                 {"released": [...]}   freeze the page again
DELETE /api/links/<id>                       revoke
GET    /s/<token>                            the link; POST /s/<token>/password, GET /s/<token>/f/<file>, POST /s/<token>/upload
GET    /s/<token>/p/<page>/attachments/<id>  an attachment the frozen page shows
```

## Export and import

The export holds what the exporter may read, a deny included, and writes the preset of the tenant on an object as
`access` (`none` on a shared collection at the top that grants the tenant nothing) and `inherit: false` where an object
breaks the inheritance. Entries of persons and groups, entries of the tenant that are no preset (custom) and denies are
not written yet, since accounts and groups differ between instances; an import gives the tenant what `access` says, and
edit at the top of an export made before ADR-29.

## From ADR-20

Migration 022 turns each level for all others into grants: a level at the top, or one wider than the level above it,
becomes a grant to the tenant; a narrower one breaks the inheritance and grants the tenant that level. The folder of the
tenant in Files grants read to the tenant and edit to its group of writers. Migration 025 adds what 022 left out: the
writers keep edit in the folders below that a narrower level made break the inheritance, as they had under ADR-20. Nobody
sees more or less than before, except that the owner of an object now manages what lies below it.

## From ADR-29

Migration 030 turns each grant of a role into allow entries of its preset, with who granted it and when; an object that
broke the inheritance still breaks it, and groups and their members stay. Nobody holds more or less than before. It also
makes the tables of nested groups, group admins and agent limits, and the search source moves to version 6, so every
index is filled again once.
