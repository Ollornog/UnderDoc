# Rights

Who sees and changes what, for documents and files alike ([ADR-29](../backlog/ADR-29-roles-on-groups-and-persons.md)).
It replaces the one level for all others of ADR-20; the owner, the private collections and the rules for comments stay.

## Roles

| Role | May |
|---|---|
| **Read** | see and open, read comments, embed what it reads |
| **Comment** | add comments and replies, on the whole page or on one block |
| **Edit** | change the content, title and tags, make, move and copy children, move to the trash and restore |
| **Manage** | grant and take back roles up to manage, break and restore the inheritance |
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

- A new collection at the top grants edit to the whole tenant, as before; its owner can change that.
- A move keeps the object's own grants and its owner; what it inherits changes with its new place.
- **Private stays private:** My notes, the folder of each user in Files and everything below them belong to their user
  alone. Nothing can be granted there, and administrators see nothing of it.
- **Embedding bypasses nothing:** an embedded page or block shows "not to be found" to whoever holds no role on it.

## Administrators

Administrators of the tenant ([sign-in.md](sign-in.md)) **manage rights, not content.** They read and change the roles of
every object that is not private, break and restore its inheritance and transfer its ownership, and they make local
groups and manage their members. They read the content of an object only where they hold a role.

## In the workspace

- **Rights and sharing** in the settings of a page and in the overview of a collection opens the rights dialog: the
  owner and the transfer of ownership, whether it takes the roles from above, the roles granted here (to everyone, a group
  or a person, changed or taken back in place), what it inherits, and its share links with a form for a new one. Whoever
  only reads sees the same, without the means to change it.
- **The facts panel** of a page names its owner and who sees it with which role ("Visible to").
- **The tab Rights** in nav1: **Groups** (administrators make local groups and take members in and out; groups from the
  sign-in are marked and read only; everyone else sees the groups they belong to), **Users** (administrators see every
  account with how it signs in and its groups, make a local account an administrator or take that back, and ask what an
  account may do), **Share links** (administrators all of them, everyone else theirs and those on what they manage, each
  revoked in place), and the **Access check** ("what may someone do": where an account holds a role of its own, through a
  group or as everyone; "who sees something": the roles of an object, and the role of an account on it with why).
- **The administration in the profile** shows Users and groups the same way (ADR-16).
- **Taking back an administrator** works for local accounts, by another administrator, so a tenant always keeps one;
  accounts of the gate or of OIDC administer through their group at the sign-in. Whoever stops administering the tenant
  stops administering its organization in the time tracking where UnderDoc made them do so
  ([time-tracking.md](time-tracking.md)).

```text
GET  /api/rights/check?object=<id>&account=<id>   the role of an account on an object, and why
GET  /api/rights/held[?account=<id>]               where an account holds a role of its own
GET  /api/users                                    administrators: every account with its source and administration
PUT  /api/users/<id>/admin                         {"admin": false}   administrators, local accounts, not oneself
```

## One check

One function in the database, `object_role`, decides the role of an account on an object, and every query that lists, finds
or opens objects asks it: the tree, search, embeddings, references, the trash, exports and the API. The search index keeps
who may read a record as tokens (`all` for the tenant, `account:<id>`, `group:<id>`, from `object_readers`); a change of
roles, of the inheritance or of the owner queues the object and everything below it.

The tables `groups`, `group_members`, `grants`, `share_links`, `share_link_users` and `share_link_passes` carry their tenant under row-level security like all others
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
restores the inheritance, and at the top, where nothing is above, it means edit as before.

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
| **read** | anyone with the link | a page **frozen**; a file, or a folder of the files with what is below it, as it is now |
| **upload** | anyone with the link | into a folder of the files: each file arrives under a name nothing there has, as a file of whoever made the link; the visitor sees nothing of the folder |
| **comment**, **edit** | accounts of the tenant | whoever opens the link while signed in holds that role on the object and below it, as long as the link lives; without an account, the link asks to sign in |

- **Frozen:** a page is rendered when the link is made, as its maker sees it then, and the link shows exactly that until a
  manager renews it. Every page it embeds, directly or through what it embeds, is listed (`/shareables`) and shows only
  if it was released; the rest says "Not shared". A collection or folder of the documents gets no link to read.
- **Listed and revoked:** `/api/links` lists the links with who made them, when, until when and how many accounts opened
  them; administrators see every link, everyone else the links they made and those on what they manage. Whoever made a
  link, whoever manages its object and administrators revoke it.
- Not yet: attachments of a page, such as a pasted image, are not part of a link, and wiki links in a frozen page lead
  into the workspace.

```text
GET    /api/objects/<id>/shareables          the pages a page embeds, to release one by one
POST   /api/objects/<id>/links               {"mode", "expires": "2026-12-31", "password", "released": ["<id>"]}  → the link with its address
GET    /api/links[?object=<id>]              the links the viewer may see
POST   /api/links/<id>/renew                 {"released": [...]}   freeze the page again
DELETE /api/links/<id>                       revoke
GET    /s/<token>                            the link; POST /s/<token>/password, GET /s/<token>/f/<file>, POST /s/<token>/upload
```

## Export and import

The export writes the role of the tenant on an object as `access` (`none` on a shared collection at the top that grants
the tenant nothing) and `inherit: false` where an object breaks the inheritance. Grants to persons and groups are not
written yet, since accounts and groups differ between instances; an import gives the tenant what `access` says, and edit at
the top of an export made before ADR-29.

## From ADR-20

Migration 022 turns each level for all others into grants: a level at the top, or one wider than the level above it,
becomes a grant to the tenant; a narrower one breaks the inheritance and grants the tenant that level. The folder of the
tenant in Files grants read to the tenant and edit to its group of writers. Nobody sees more or less than before, except
that the owner of an object now manages what lies below it.
