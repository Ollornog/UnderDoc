---
id: ADR-29
type: Decision
title: Roles for persons, groups and the tenant, the same for documents and files
status: erledigt
tags: [permissions, security, architecture]
created: 2026-11-19
---

## Context

ADR-20 gave every object an owner and one level for all others. M-5 asks for roles per person and group, shares by link and
a log, good enough for company data. The documents and the view Files (ADR-28) are two trees, and T-9 asked which one decides
the roles a document inherits. The maintainer set the frame on 2026-11-16:

- Rights hold globally on groups, the same for documents and data.
- There are local groups and groups taken over from OIDC or the gate; those taken over can only be read. Groups do not nest
  for now.
- The roles are owner, manage, edit, comment and read. There is only allowing; what must be narrower breaks inheritance.
- Shares go to persons, groups, the whole tenant, or a link with a token (expiry, password, and read, comment, edit or
  upload only).
- Groups and single persons alike, on every level down to the single object: page, file, folder.
- Samba knows rights per single file only as far as T-1 proves them; otherwise the interface shows that Samba knows them
  per folder only.
- A tab "Rights" in nav1 with groups, users, share links, an access check ("what may X do", "who sees Y") and the log.
  Administrators see everything there, everyone else what is theirs.

## Options

**Which tree decides (T-9):**

- **The workspace decides:** matches how people think, but the share may show less or more than the web.
- **The files tree decides:** the share and the web always agree, but arranging a document never changes who sees it.
- **The workspace decides, and folders shared over Samba or WebDAV set a limit** that the interface shows. This was the
  starting recommendation of T-9.

**How a narrower right is expressed:**

- **Deny entries:** flexible, but every reader has to work out which entry wins, and the log no longer says plainly who
  may do what.
- **Allow only, and inheritance can be broken:** what holds is the sum of what is granted; narrower means not taking from
  above. This is the frame of the maintainer.

**What administrators may do:**

- **Administrators hold every role on everything that is not private** (as ADR-20 said, though never built): simple, but
  every administrator reads every document of the company.
- **Administrators manage rights, not content:** they see who holds which role on every object that is not private, change
  roles and transfer ownership, and see the content only where they hold a role themselves, which the log shows.

## Decision

### Which tree decides (T-9)

**The tree an object lives in decides, and folders shared over Samba or WebDAV set a limit that the interface shows.** This
is the starting recommendation, made concrete by the split of ADR-28: a document lives in the workspace, a file in the view
Files, and each object has exactly one place (ADR-13). A reference to a file in the workspace, a link or an embedding grants
nothing and carries no role: whoever has no role on the source sees "not to be found" (ADR-18).

The limit of a share: Samba gets rights per folder, and per single file only where T-1 shows that they survive a save.
Until then, in a folder that is shared:

- a role granted on a single file holds in the web only, and the rights dialog says "Samba knows rights per folder only";
- breaking the inheritance of a single file is refused, so the share never shows a file that the web hides; a folder can
  break it, and becomes a folder of its own for Samba.

The shares come with the Samba part of M-5, and this limit with them.

### Who can hold a role

- **A person:** an account of the tenant.
- **A group:**
  - **Local groups** are made by administrators, who add and remove their members.
  - **Groups taken over** from OIDC (the claim of ADR-23) or the gate come from the sign-in: each name seen in a sign-in is
    a group, and its members are the accounts whose last sign-in named it. They can only be read here.
  - Groups do not nest. A local group and a group taken over may share a name; the interface marks where each comes from.
- **The tenant:** every account of the tenant.
- **A link** with a token (below).

### The roles

From the least to the most, each including those below it:

| Role | May |
|---|---|
| **Read** | see and open, read comments, embed what it reads |
| **Comment** | add comments and replies, on the whole page or on one block (ADR-18, ADR-20) |
| **Edit** | change the content, title and tags, make and copy children, reorder within the parent, move to the trash and restore, see versions |
| **Manage** | grant and take back roles up to manage, break and restore inheritance, make and revoke share links, move to another place |
| **Owner** | transfer the ownership; nobody takes the role away |

- **One owner per object,** a person: whoever made it. The owner of an object manages everything below it while that
  inherits.
- **Ownership is transferred** by the owner or an administrator, to another account of the tenant. The former owner keeps
  the role edit, which can be taken back like any other.
- **Groups and persons alike, on every level:** a role can be granted on a collection, a folder, a page, a board of notes,
  a root or a folder of the files, or a single file.

### Inheritance

- **An object holds what is granted on it and on everything above it**, up to the top or to an object that breaks the
  inheritance. Of all roles a person holds through themselves, their groups, the tenant and the links they used, the highest
  counts.
- **There is only allowing.** To narrow, an object **breaks the inheritance**: it no longer takes roles from above. At that
  moment the roles that held from above are copied onto it, so nobody loses access by surprise; then the manager takes back
  what shall not hold. Restoring the inheritance takes from above again and keeps the object's own grants.
- **A move** keeps the object's own grants and the owner; what it inherits changes with its new place. Within its parent,
  edit reorders it; to another place it goes only with manage on the object and edit on where it goes (2026-11-19, until
  then edit on both). Only its owner moves an object into something private, and only when everything below it is theirs.
- **A new collection at the top** grants edit to the whole tenant, as the default of ADR-20 did, and its owner can change
  that right away. Everything else starts with what it inherits.

### Private stays private

My notes, the folder of each user in Files and the private area belong to their user alone: no role can be granted there,
no link made, and administrators see nothing of it. Whatever lies below a private object is private as well.

### Administrators

Administrators of a tenant (ADR-23) **manage rights, not content.** In the tab Rights and in the rights dialog they see and
change the roles of every object that is not private, break and restore inheritance and transfer ownership. They read the
content of an object only where they hold a role, and granting themselves one is logged like every change. They make
local groups, manage their members, see every share link and can revoke it, and see the whole log.

An administrator who is one through a local account can be made an ordinary user again by another administrator; the last
administrator of a tenant cannot. Whoever stops being an administrator also stops administering the organization of the time
tracking (M-24), where UnderDoc made them one.

### Links

- A link shares one object and what is below it, through a token of 32 random bytes in its address; only a hash of the
  token is stored. It has an optional expiry and an optional password (Argon2id, with the limit against guessing of the
  sign-in), and shows who made it and when it ends. Links can be listed and revoked; a revoked or expired link answers like
  one that never was.
- **Read** is for anyone with the link, without an account. **A page is shown frozen:** the link keeps the page as it was
  rendered when the link was made or last renewed by a manager. Every object the page embeds is listed when the link is
  made, and only those the maker releases are part of it; the rest shows "not shared". Values and queries show the result
  of that moment, never a live query. A file shows its current version, a folder of the files its current files.
- **Upload only** is for a folder of the files: anyone with the link adds files and sees none.
- **Comment** and **edit** are for accounts of the tenant: whoever opens such a link while signed in holds that role on the
  shared object from then on, as long as the link lives. Without an account they read.
- Only a manager makes a link, never on something private, and never above what they hold.

### Every check in one place

- **One function decides the role** of an account on an object, in the database, and every query that lists, finds or
  opens objects asks it: the tree, search, embeddings, references, the trash, exports and the API. A query cannot forget
  it, since what an account may not see never leaves package `db` (as ADR-20 began).
- The search index keeps who may read a record as tokens of persons, groups and the tenant; a change of roles, of
  inheritance or of the owner reindexes the object and what is below it.
- Each new table carries its tenant under row-level security (ADR-23) and is part of the guard test.

### The log

Changes of rights, ownership, groups, administrators and links are logged each time; opening and changing an object once
per account, object and hour; the use of a link and each wrong password of a link each time. Administrators see the whole
log in the tab Rights, everyone else what they did and what happened to the objects they manage.

### The tab Rights

nav1 gets the tab Rights: groups, users, share links, the access check ("what may X do", "who sees Y") and the log.
Administrators see all of it; everyone else sees the groups they belong to, the links they made, the access check for the
objects they manage and their part of the log. Each document, folder and file has a rights dialog, and its facts panel names
the owner and who sees it.

## Consequences

- **ADR-20 is replaced in its level for all others;** its owner, its private collections and its rules for comments stay. A
  migration turns each level into grants: a level at the top, or a level wider than the one above it, becomes a grant to the
  tenant; a level narrower than the one above it breaks the inheritance and grants the tenant that level. The folder of the
  tenant in Files grants read to the tenant and edit to its group of writers (ADR-28). Nobody sees more or less than before,
  except that the owner of an object now manages what lies below it.
- T-9 is settled by this decision; T-1 can only widen the share to rights per single file.
- The Samba part of M-5 (users, shares, the domain) and the limit of a share come last, with T-1.
- Passwords in the text (ADR-7) are a piece of their own; this decision gives them no rule beyond ADR-7.
- **Built on 2026-11-19** ([docs/rights.md](../docs/rights.md), migrations 022 to 024). Settled in the build:
  - The level of ADR-20 in the settings of the workspace stays as the role of the whole tenant; "as above" at the top
    means edit, as it did.
  - A link lives only while its maker manages its object; otherwise it answers like one that never was and lends no
    role. Administrators who do not manage an object make no link to it, but see and revoke every link.
  - A link to read shares a page, a file, or a folder of the files with what is below it; a collection or folder of the
    documents is shared with accounts only, to comment or edit. A frozen page holds the page alone, not the pages below
    it, and not its attachments yet.
  - An upload through a link becomes a file of the link's maker under a name nothing in the folder has.
  - Nobody takes their own administration away, so a tenant keeps an administrator. The organization of the time tracking
    follows only for memberships UnderDoc made itself (ID prefix `ud-admin-`), those from before stay.
  - The log keeps opening and changing once per account, object and hour, keeps nothing private, and has no retention
    period yet.
- **Settled after an attack round on 2026-11-19** (each with a test that fails without it):
  - **Moving to another place needs manage** on the object and edit on where it goes; reordering within the parent stays
    with edit. Before, edit on both was enough, and whoever only edited a page moved it into a collection of their own,
    managed it there as the owner above and could share it more widely. The workspace opens no gap for such a move.
  - **Into something private** goes only what is wholly the mover's: something of someone else below it refuses the move
    with a message. Its grants and links end with the move, and the log keeps each for the owner. "What may X do"
    leaves out everything private.
  - **Deleting stops at what shuts the deleter out:** whoever deletes or purges must be allowed to edit everything that
    would go with it. A page below that breaks the inheritance and shuts them out refuses the deletion with a message,
    and its owner moves it out first. This was chosen over moving such objects to the next place above their owner can
    reach: it is simpler, and nothing changes its place without its owner.
  - **Versions are for edit,** as the table says, for pages and files alike; whoever reads a file gets its current
    version.
  - **A link never shows more than its maker sees now,** checked for each object it reaches: a folder below that breaks
    the inheritance and shuts the maker out is neither listed nor opened through it, nor are its files.
  - **An upload through a link takes its name when the file is made,** with the folder locked, and never becomes a version
    of a file that arrived under that name meanwhile.
  - **The path of a search result** names only the levels above that every reader of the record sees; the others show as
    "…".
  - **The writers of the folder of the tenant keep edit in its narrower folders:** migration 022 broke their inheritance
    and copied nothing; migration 025 grants the group edit on each such break, so they edit where they did under
    ADR-20.
  - **Taking back an administrator is decided in the database,** under a lock: a local actor must still administer, so
    two administrators who take each other's role at the same moment never leave the tenant without one.
  - **Open, for the maintainer:** whoever reads an object may copy it and owns the copy, and so may share the copy by a
    link. Whether reading should allow copying, or a copy keep the rights of its source, is not decided.
