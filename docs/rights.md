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
| **Members** | read, comment, create, write, share, manage: everything but delete (T-34) |
| **Manage** | all seven |
| **Owner** | all seven, and transferring the ownership and archiving a space; nobody takes it away |

**Members** manages who may do what (members and rights) and edits, but deletes nothing and archives nothing; **Manage**
keeps delete. The team wiki's *moderator* is Members ([T-28](../backlog/T-28-import-and-export-of-a-git-markdown-wiki-with-a-ro.md)).

**Nobody gives what they do not hold** (T-34): whoever manages an object only opens up the actions they hold there
themselves, that is allows one or takes back a deny of one, also for the whole tenant and through the level of the
workspace (`/access`). Narrowing is always allowed: taking an allow back, denying. So a holder of Members gives Read,
Comment and Members, takes any role back, but gives no Edit or Manage (`403`). An entry holds below the object too,
wherever the inheritance is not broken, so the actions it opens up must be held on each of those as well: a manager with
a deny of delete on a page inside gives nobody delete on the folder around it (B-6). The entries are read under a lock
of the object, in the transaction that changes them, so a deny set at the same moment is seen. **Restoring the inheritance** may open up
anything from above, so only whoever holds all seven actions there does it; breaking it copies what holds and is open to
every manager. A **link** to comment or edit lends only the actions its maker still holds on the object asked for, as
long as the maker may still share it there (B-6). Administrators manage rights whatever they hold.

**The owner** is one person per object, whoever made it. Entries go to

- **a person:** an account of the tenant;
- **a group:** a **local group**, or a **group taken over** from the gate (its roles in `Remote-Groups`):
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
  admins, name, and making a new group inside it. Administrators of the tenant manage all groups.
- The interface says "group" for both kinds and marks where each comes from.

### Archived groups

A local group is **archived, never deleted** (B-3). It keeps its members, the groups inside it, its admins and every
entry granted to it, and **Restore** brings all of it back. Nothing in UnderDoc deletes a group for good.

- **Who:** administrators of the tenant and delegated administrators (B-5, T-33). Group admins never
  archive or restore a group, not even one inside theirs: through nesting (put a group inside, take oneself out as its
  admin, archive, restore, take oneself in again) any rule for them could be walked around. Agents never do.
- **When:** only while every group inside it is archived already (or taken out); otherwise `409`. A group comes back
  only while no group it is inside is archived.
- **What holds while it is archived:**
  - its **allow** entries hold for nobody, and nothing passes up through it: its members do not count as members of the
    groups it is inside;
  - its **deny** entries keep holding for whoever is in it, also through the groups it is inside. Archiving never gives
    anyone more: if a deny went with the group, the next entry above could allow what it denied. An allow that holds
    for nobody only lets the decision go on up, which never allows more than the allow did;
  - restricted passwords ([secrets.md](secrets.md)) are not revealed through it; its admins manage nothing through it;
  - it gets no new entry, member, admin or group inside, is put inside no group, becomes no new reader of a password,
  and is offered nowhere for a grant. Its entries can still be taken back, and a reader of a password it already is stays
  while the list is written anew (B-4).

## Archived spaces

A space is **archived** instead of left in the way or deleted (T-34). Only the space itself, a collection at the top of the
documents or of Files, is: a space has one owner who answers for it; what lies inside one is moved to a space of its own
first. The folder of the workspace and the folders of users among Files, and private notes, are never archived.

- **Who:** its **owner** or an **administrator**, in a **browser session** (`Remote-Auth: session`), never an agent, a
  service account, a key or an app password, as deleting for good. Neither Manage nor Members archives, nor do group
  admins. Restoring is for the same people. The workspace offers it in the overview of the space (`archive` in
  `/api/state`).
- **Kept:** everything inside it, its entries and its links, **without a limit**: the archive is not the trash, and nothing
  goes by itself. What already lay in the trash of the space waits while it is archived, since nothing of it can be
  restored then: its 30 days pause, and restoring the space gives them back (B-6).
- **Hidden:** the tree (`/api/state`, `/api/files`), the search and the tree of MCP leave archived spaces out, unless
  asked for (`archive=1`, the button *Show the archive* below the collections and below the roots of Files, the filter
  *Also in the archive* of the results page, `archived` of the MCP tools `context` and `search`), or unless the search
  names the space as its collection. An address still opens what lies inside, for whoever may read it.
- **Read only for everyone**, the owner and administrators included, until it is restored: the one check refuses
  comment, create, write, delete and share inside it (`archived` as the deciding entry of the access check). So every door
  holds it at once: the web, the API, MCP and its upload links (also one made before), Samba (read only for everyone),
  uploads in the browser (also one started before), share links (a link lends nothing and opens nothing while its maker
  may not share, which nobody may in the archive; restoring brings them back unless they expired), copying in, moving in
  or out, restoring from the trash into it, renaming it. A write that checked its rights before the space was archived
  does not land in it either: every write of content (blocks, versions of files, attachments, new objects) holds the
  space in its transaction, and archiving waits for it or the write finds the archive (`holdSpace`, B-6). Read stays as
  the entries say, and so does **manage**: who may
  read the archive can still be set, opening up only read and manage (above). Copying out needs read here and create
  there. The owner and administrators still transfer it.
- **In the database** the columns `objects.archived` and `archived_by` (migration 037) mark a space; a check keeps them on
  a collection at the top that is not private. The log names archiving and restoring (`archive`).
- **An import** brings an archived space archived: an export marks it (`archived: true` in its `.underdoc.yaml`), and
  `underdoc import --archived` archives what it makes ([export.md](export.md)). Groups that arrive archived are made by
  `db.ImportGroup`, which only runs without a request, as the command line does ([T-28](../backlog/T-28-import-and-export-of-a-git-markdown-wiki-with-a-ro.md)).

```text
PUT /api/objects/<id>/archive    {"archived": true}   archives a space; false restores it: owner or administrator, session
GET /api/state?archive=1         the archived spaces of the documents too, after the others, each "archived": true
GET /api/files?archive=1         the archived collections of Files too
GET /api/search?…&archive=1      the archived spaces too
```

## Spaces

A **space** is a collection at the top, of the documents or of Files. Only **administrators** and the holders of the
role of the app to create spaces in the gate (`UNDERDOC_SPACES_ROLE`, `underdoc-spaces` unless configured,
[sign-in.md](sign-in.md#the-gate)) make one (ADR-35, B-3), also by copying a collection to the top or moving one up
there; anyone else gets `403`, and the workspace offers it nowhere (`canSpaces` in `/api/state`). No agent makes one,
also not with a key, an app password or OAuth of a holder of the role (B-5). Inside a space, the rights decide as everywhere. The command line (`underdoc import`) makes
collections at the top as whoever runs the instance.

## The group Agents

- **Who is in it:** every request whose sign-in is not an interactive session. The way comes from the gate's
  `Remote-Auth` (`session`, `oauth`, `apikey`, `app-password`), and a missing or unknown value counts as an agent (fail closed). The value comes only from the result
  the gate's Go client checked ([sign-in.md](sign-in.md#the-gate)), never raw from a header. In the code it is
  `account.User.AuthKind`, and `account.User.Agent()` decides.
- **Service accounts** are always in it (B-3, [below](#service-accounts)).
- **A deny on Agents is a ceiling** after the evaluation above: an action denied for Agents on the object or anywhere
  above it is refused, for the owner too, and breaking the inheritance below it does not lift it.
- **Agents never** manage rights or groups, share (links), delete for good (empty the trash) or read passwords
  (ADR-7). Read, create, write, comment and moving to the trash stay as their person may.
- **Agents never publish** (B-5): they move or copy nothing, a page, a folder or a block, to where someone may read it
  who may not read it where it is now (`403`); a person does that. A copy keeps of what lies below its top none of its
  entries for persons or groups, its denies or a break of its inheritance, so every object a copy makes is checked, not
  only its top (B-6). A live link that shows its object (read, comment or edit, not upload) on the target or above it
  counts as readers without limit, and a link on what an agent moves that is dead where it lies, because its maker may
  not share it there, counts where its maker may share the target (B-6). They make no space and move no space to the trash,
  and the **rules page** ([mcp.md](mcp.md#the-rules-page)) they only read, whatever its entries say (deciding entry
  `rules page`).
- **What they write** where more people read than read what they read is marked for a person to check, and every write
  names the readers of its target ([mcp.md](mcp.md#who-reads-what-an-agent-writes), T-38). Agents offer no page as a
  template and make no page from a template fewer may read than the new page's readers.
- **No agents** on a space is a deny on read for Agents there, set like any entry (kind `agents`).
- **Every person limits their own agents** (`PUT /api/me/agents`), for example to read only; only the person in a
  session changes it, never an agent.

### Service accounts

A service account (`Remote-Kind: service`) comes in only with a lasting ID an administrator allowed
(`UNDERDOC_SERVICE_ACCOUNTS`, [sign-in.md](sign-in.md#the-gate)); any other is refused (B-3). Then:

- it is **always an agent**, whatever a request or a caller says: the one check (`object_decide`) takes every service
  account for an agent, so its ceilings hold in the web, the API, the search, the export and on the share;
- it **never belongs to everyone signed in**: an allow for the tenant never holds for it, and neither does an allow for a
  group taken over from the gate, since a role such as the one that lets people use UnderDoc means everyone too. It sees
  the folder source empty;
- it gets what is **granted to it**, and to a **local group** it is put in directly (not through a group taken over
  inside one), and to the local groups that group is inside, as for anyone (B-4);
- a **deny** for the tenant or for a group taken over still holds for it;
- it **administers nothing and makes no space**, whatever roles the gate gives it (B-4): the sign-in does not read the
  roles of the app for it, and the checks of the administration and of spaces refuse it as well.

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
- **Deleting for good** from the trash (B-3, ADR-35) is for its **owner** or an **administrator** only, in a **browser
  session** (`Remote-Auth: session`, never an agent, a key or an app password), with its **name typed in**, which the
  server checks exactly; emptying the trash needs `Trash` typed in and takes only what the person may delete for good.
  Whoever only deleted it waits for its owner, or for the trash to empty itself after 30 days. Once the gate signs
  approvals for actions of an app, with `UNDERDOC_APPROVALS=on` it needs one as well (T-33,
[accounts.md](accounts.md#deleting-for-good-with-an-approval), `db.SetApprover`): what goes is locked and checked
  first, then the approval is asked for, then it goes, nothing in between, so an approval is used up only by what goes.
  Emptying the trash asks for one approval bound to a cutoff, and takes only what was deleted at or before it (B-6).
- **Versions are for write:** the history of a page or a file holds what was taken out, so whoever only reads gets the
  current state, not the versions.
- **Embedding bypasses nothing:** an embedded page or block shows "not to be found" to whoever may not read it, a deny
  included; an
  embedded table of a spreadsheet the same, without the name of its file ([tables.md](tables.md#tables-in-pages)).
- **Passwords in the text** ([secrets.md](secrets.md), ADR-7) are revealed by whoever reads their page, or narrower by
  whom its managers name; a role lent by a share link reveals none.

## Known limits

What the rights do not do, on purpose or not yet (B-6):

- **The search may trail a new deny** for a moment: its counts come from the index until the queue is drained; the hits
  themselves are checked every time.
- **Members on Samba:** a holder of Members writes in the browser, but the share knows rights per folder only and shows
  them read only there.
- **An import** (`underdoc import --archived`, the command line only) archives without the check of the fixed roots the
  web makes; the check of the database (a collection at the top that is not private) still holds.
- **Group admins pass on through membership** what they do not hold themselves: putting someone into a group gives them
  what the group holds. The rule "nobody gives what they do not hold" covers entries, not memberships.
- **A copy keeps the tags**, so an agent can make a page tagged `rules`; since B-5 the tag makes no rules page.
- **"Agents never publish"** covers moving, copying, moving blocks and templates. An agent that reads something and
  writes it anew elsewhere is not stopped, but such a write is marked ([T-38](../backlog/T-38-attack-round-an-agent-makes-something-readable-to.md));
  what a person told it in the chat no mark can see, which is why every write names its readers.
- **Versions** keep what was taken out of a page. Whoever gets write on it later reads them, also what was removed before
  the page was opened up (T-38, open for the maintainer).
- **Restoring from the trash** brings an object back where it lay, with the readers that place has now, also when they
  grew while it lay in the trash (T-38, open for the maintainer).
- **Files** an agent downloads and uploads again as a new file are a new file: nothing tells it apart from one written
  anew (T-38, open for the maintainer).

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

**Delegated administrators** (`UNDERDOC_DELEGATE_ROLE`, T-33) manage the accounts of every group through the gate,
see the users and the members of every group, and archive and restore groups; nothing else of an administrator, and never
the account of an administrator ([accounts.md](accounts.md#two-levels-of-administration)).

**Transferring the ownership** is for the owner and administrators, in a browser session only (T-34): never an agent, a
service account, a key or an app password.

**Locked accounts** (T-33): an account UnderDoc locked comes in through no way and may do nothing, the one check refuses
it every action (deciding entry `locked`), and its links lend nothing until it is unlocked
([accounts.md](accounts.md#the-lock-of-underdoc)).

**What someone left behind:** the gate or the identity provider ends the sign-in, or UnderDoc locks the account. An
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
- **Administrators** hold the role of the app in the gate ([sign-in.md](sign-in.md#the-gate)). **Taking back** works
  for the mark the command line gives in an emergency, by another administrator, never one's own; the database decides it
  under a lock, and an administrator without the role must still be marked when it happens. The web marks nobody. Whoever stops administering the tenant
  stops administering its organization in the time tracking where UnderDoc made them do so
  ([time-tracking.md](time-tracking.md)).

```text
GET  /api/rights/check?object=<id>&account=<id>[&agent=1]   each action of an account (or its agent) on an object, with
                                                   the deciding entry: via, object, kind, who, value
GET  /api/rights/held[?account=<id>]               where an account holds entries of its own that hold for it: an allow
                                                   through an archived group, and for a service account one for the
                                                   tenant or a role of the gate, is left out, its denies stay (B-4)
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
PUT    /api/objects/<id>/grants              {"kind": "person|group|tenant", "who": "<id>", "role": "read|comment|edit|members|manage|"}
PUT    /api/objects/<id>/grants              {"kind": "person|group|tenant|agents", "who": "<id>",
                                              "actions": {"read": "allow", "write": "deny"}, "subgroups": true}
PUT    /api/objects/<id>/inherit             {"inherit": false}   break; true restores
PUT    /api/objects/<id>/owner               {"to": "<account id>"}   owner or administrator, in a browser session
PUT    /api/objects/<id>/archive             {"archived": true|false}   a space: owner or administrator, session (above)
POST   /api/objects/<id>/access              {"access": "edit|comment|read|"}   the level for the whole tenant (ADR-20)
GET    /api/people                           accounts and groups to grant roles to
GET    /api/groups                           administrators: all; everyone else: their own and those they manage
GET    /api/groups/<id>                      the group, its persons, groups inside it, groups it is in, its admins
POST   /api/groups                           {"name", "description", "parent"}   administrators; group admins inside theirs
PATCH  /api/groups/<id>                      {"name", "description"}   administrators and group admins, local groups
DELETE /api/groups/<id>                      archives a local group (B-3): administrators and the admins of a group it is
                                             inside, while every group inside it is archived; 409 otherwise
POST   /api/groups/<id>/restore              brings an archived group back, the same people
DELETE /api/trash/<id>                       {"confirm": "<its name>"}   deletes for good: owner or administrator, session
DELETE /api/trash                            {"confirm": "Trash"}   empties what the person may delete for good
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
and lends nobody a role. The log names `/s/…` without the token. A link to comment or edit is taken behind the gate, at
`/link/<token>` ([sign-in.md](sign-in.md#share-links)).

| Mode | For | What it does |
|---|---|---|
| **read** | anyone with the link | a page **frozen**; a file, or a folder of the files with what is below it, as it is now, and only what the maker of the link sees now, checked for each folder and file |
| **upload** | anyone with the link | into a folder of the files: each file arrives under a name nothing there has when it is made, as a file of whoever made the link, never as a version of what is there; the visitor sees nothing of the folder |
| **comment**, **edit** | accounts of the tenant | whoever opens the link while signed in holds that role on the object and below it, as long as the link lives and only where its maker still holds each action, on the object asked for itself (a deny for the maker on a page below holds for the link too, B-6); without an account, the link asks to sign in |

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
