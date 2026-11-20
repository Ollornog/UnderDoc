# Rights

Who sees and changes what, for documents and files alike ([ADR-29](../backlog/ADR-29-roles-on-groups-and-persons.md)).
It replaces the one level for all others of ADR-20; the owner, the private collections and the rules for comments stay.

## Roles

| Role | May |
|---|---|
| **Read** | see and open, read comments, embed what it reads |
| **Comment** | add comments and replies, on the whole page or on one block |
| **Edit** | change the content, title and tags, make and copy children, reorder within the parent, move to the trash and restore, see the versions |
| **Manage** | grant and take back roles up to manage, break and restore the inheritance, make share links, move to another place |
| **Owner** | transfer the ownership; nobody takes the role away |

Each role includes those above it in the table. **The owner** is one person per object, whoever made it; the owner of an
object manages everything below it while that inherits. A role is granted to

- **a person:** an account of the tenant;
- **a group:** a **local group**, made by administrators, who add and remove its members; or a **group taken over** from
  OIDC (the groups claim) or the gate (`Remote-Groups`): each name a sign-in brings is a group, and its members are the
  accounts whose last sign-in named it. A group taken over can only be read here. Groups do not nest;
- **the tenant:** every account of the tenant.

## Inheritance

An object holds what is granted on it and on everything above it, up to the top or to an object that **breaks the
inheritance**. Of all roles an account holds through itself, its groups and the tenant, the highest counts. There is only
allowing: to narrow, break the inheritance. Breaking copies the roles that held from above onto the object, so nobody loses
access by surprise; then take back what shall not hold. Restoring the inheritance takes from above again and keeps the
object's own grants.

- **A new collection at the top belongs to its owner alone** until they share it (2026-11-19; before, it granted edit to
  the whole tenant): no role for anyone else, not even the tenant. It is not private, so it is shared like any other. The
  dialog that makes it has the step **Share with**: everyone in the workspace, groups and persons, each with a role, by
  default nobody; it grants through `PUT /api/objects/<id>/grants`, as the rights dialog does. The same holds for a new
  collection of Files and for a copy made at the top; a collection inside another inherits. Collections made before keep
  what they granted.
- A move keeps the object's own grants and its owner; what it inherits changes with its new place. **Moving to another
  place needs manage** on the object and edit on where it goes; within its parent, edit reorders it. The workspace
  opens no gap for a move one may not make.
- **Private stays private:** My notes, the folder of each user in Files and everything below them belong to their user
  alone. Nothing can be granted there, and administrators see nothing of it. Only its owner moves an object into
  something private, and only when everything below it is theirs too; otherwise the move is refused with a message,
  and nothing new comes in anywhere below while that is checked. Its
  grants and links end with the move, and the log keeps each for the owner.
- **Deleting takes nothing one may not edit:** deleting or purging needs edit on the object and everything below it. A
  page below that breaks the inheritance and shuts the deleter out refuses the deletion with a message; its owner moves
  it out first.
- **Versions are for edit:** the history of a page or a file holds what was taken out, so whoever only reads gets the
  current state, not the versions.
- **Embedding bypasses nothing:** an embedded page or block shows "not to be found" to whoever holds no role on it; an
  embedded table of a spreadsheet the same, without the name of its file ([tables.md](tables.md#tables-in-pages)).
- **Passwords in the text** ([secrets.md](secrets.md), ADR-7) are revealed by whoever reads their page, or narrower by
  whom its managers name; a role lent by a share link reveals none.

## On the Samba share

In a library shared over Samba ([samba.md](samba.md), ADR-30) roles become ACLs per folder: read and comment read, edit
and above write. Samba knows rights per folder only (T-1): a role granted on a single file holds in the web only, and
the rights dialog says so; breaking the inheritance of a single file there is refused (`409`), a folder can break it. A
folder in which someone holds a role without reading the folder above is a share of its own for them.

## Administrators

Administrators of the tenant ([sign-in.md](sign-in.md)) **manage rights, not content.** They read and change the roles of
every object that is not private, break and restore its inheritance and transfer its ownership, and they make local
groups and manage their members. They read the content of an object only where they hold a role.

**What someone left behind:** UnderDoc deactivates no account; the gate or the identity provider ends the sign-in. An
administrator asks "what may they do" for that account in the tab Rights, which lists what it owns by title, and
transfers each in place to another account, which the form leaves unchosen until the
administrator chooses. Transferring shows the administrator nothing of the content, unless they
give it to themselves; the log keeps every transfer (`owner`).

## In the workspace

- **Rights and sharing** in the settings of a page and in the overview of a collection opens the rights dialog: the
  owner and the transfer of ownership, whether it takes the roles from above, the roles granted here (to everyone, a group
  or a person, changed or taken back in place), what it inherits, and its share links with a form for a new one. Whoever
  only reads sees the same, without the means to change it.
- **The facts panel** of a page names its owner and who sees it with which role ("Visible to").
- **The tab Rights** in nav1: **Groups** (administrators make local groups and take members in and out; groups from the
  sign-in are marked and read only; everyone else sees the groups they belong to), **Users** (administrators see every
  account with how it signs in and its groups, make a local account an administrator or take that back, and ask what an
  account may do, with what it owns transferred in place), **Share links** (administrators all of them, everyone else theirs and those on what they manage, each
  revoked in place), and the **Access check** ("what may someone do": where an account holds a role of its own, through a
  group or as everyone; "who sees something": the roles of an object, and the role of an account on it with why).
- **The administration in the profile** shows Users and groups the same way (ADR-16).
- **Taking back an administrator** works for local accounts, by another administrator, so a tenant always keeps one;
  the database decides it under a lock, and a local administrator must still be one when it happens;
  accounts of the gate or of OIDC administer through their group at the sign-in. Whoever stops administering the tenant
  stops administering its organization in the time tracking where UnderDoc made them do so
  ([time-tracking.md](time-tracking.md)).

```text
GET  /api/rights/check?object=<id>&account=<id>   the role of an account on an object, and why
GET  /api/rights/held[?account=<id>]               where an account holds a role of its own
GET  /api/users                                    administrators: every account with its source and administration
PUT  /api/users/<id>/admin                         {"admin": false}   administrators, local accounts, not oneself
```

## The log

Access and changes are logged ([ADR-29](../backlog/ADR-29-roles-on-groups-and-persons.md), migration 024, table
`audit_log`): opening an object and changing it (each change of the API that went through, an upload, making something)
once per account, object and hour; roles, the inheritance, owners, groups, members, administrators and links each time,
with what was done; a link opened, used, a wrong password of a link and an upload through a link, with an empty actor for
someone without an account; grants taken back and links revoked by a move into something private, visible to its owner; and each time a password of
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

One function in the database, `object_role`, decides the role of an account on an object, and every query that lists, finds
or opens objects asks it: the tree, search, embeddings, references, the trash, exports and the API. The search index keeps
who may read a record as tokens (`all` for the tenant, `account:<id>`, `group:<id>`, from `object_readers`); a change of
roles, of the inheritance or of the owner queues the object and everything below it. The path of a record names only the
levels above that every one of its readers sees; the others show as "…".

The tables `groups`, `group_members`, `grants`, `share_links`, `share_link_users`, `share_link_passes`, `audit_log` and `log_settings` carry their tenant under row-level security like all others
([tenants.md](tenants.md)). The column `objects.inherit` says whether an object takes roles from above.

## API

```text
GET    /api/objects/<id>/rights              owner, own grants, inherited grants, the viewer's role
PUT    /api/objects/<id>/grants              {"kind": "person|group|tenant", "who": "<id>", "role": "read|comment|edit|manage|"}
PUT    /api/objects/<id>/inherit             {"inherit": false}   break; true restores
PUT    /api/objects/<id>/owner               {"to": "<account id>"}
POST   /api/objects/<id>/access              {"access": "edit|comment|read|"}   the level for the whole tenant (ADR-20)
GET    /api/people                           accounts and groups to grant roles to
GET    /api/groups                           administrators: all; everyone else: their own
POST   /api/groups                           {"name", "description"}   administrators
PATCH  /api/groups/<id>                      {"name", "description"}   administrators, local groups
DELETE /api/groups/<id>                      administrators, local groups; their grants go with them
GET    /api/groups/<id>/members              administrators and members
PUT    /api/groups/<id>/members/<account>    {"member": true}   administrators, local groups
```

An empty role takes a grant back. The level of ADR-20 (`/access`) stays for the settings of the workspace: a level narrower
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

The export writes the role of the tenant on an object as `access` (`none` on a shared collection at the top that grants
the tenant nothing) and `inherit: false` where an object breaks the inheritance. Grants to persons and groups are not
written yet, since accounts and groups differ between instances; an import gives the tenant what `access` says, and edit at
the top of an export made before ADR-29.

## From ADR-20

Migration 022 turns each level for all others into grants: a level at the top, or one wider than the level above it,
becomes a grant to the tenant; a narrower one breaks the inheritance and grants the tenant that level. The folder of the
tenant in Files grants read to the tenant and edit to its group of writers. Migration 025 adds what 022 left out: the
writers keep edit in the folders below that a narrower level made break the inheritance, as they had under ADR-20. Nobody
sees more or less than before, except that the owner of an object now manages what lies below it.
