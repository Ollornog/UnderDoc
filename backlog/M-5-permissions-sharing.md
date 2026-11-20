---
id: M-5
type: Milestone
title: Permissions and sharing
status: in-arbeit
tags: [permissions, security]
created: 2026-11-06
---

Permissions are good enough for company data.

**Done when this is true:**

- Every document has an owner, editors, commenters and viewers, per person or group, and ownership can be transferred.
- Parents pass default roles on to what is below them, as T-9 decides, and embedding bypasses nothing.
- Commenters can comment on a document, without an editor. Comments on a whole document arrive with M-28, and here a comment on
  one block joins them (ADR-18).
- Roles per person and group replace the one level for all others of ADR-20, and the owner stays.
- Notes are private per user.
- Public shares use a token, list all embedded objects for release and only show frozen data.
- Share links can be listed and revoked, and each one shows who created it and when it expires.
- The metadata adds the owner and who can see a document.
- An administrator role sees the administration in the profile: modules, users and groups, and the rest of the system settings
  (ADR-16).
- The app manages Samba users and permissions without running as root, with a password per user. Roles reach the share as
  far as T-1 has proven them.
- Samba can optionally join a domain.
- Passwords in text work as described in ADR-7, and an attack round has looked at them.
- Access and changes are logged.

**Settled on 2026-11-16 (talk with the maintainer, sprint of that day):** Sign-in and tenants follow ADR-23. Public shares are proven in tests; on the test instance the gate at the edge stays closed, and the gate on the host lets the share path through for the mesh only. Samba users come with the Samba part at the end.

**How the criteria are cut (2026-11-19):** [ADR-29](ADR-29-roles-on-groups-and-persons.md) decides the rights and settles
T-9. The block of rights then comes in this order, each step a pull request of its own:

1. **Data model and the one check:** groups (local and taken over), grants to persons, groups and the tenant on every
   object, breaking inheritance, transferring ownership, the levels of ADR-20 migrated into grants, embedding and search
   asking the same check, notes private. Covers the first, second, fourth and fifth criterion.
2. **Share links:** token, expiry, password, the modes, the embedded objects released one by one, frozen pages; listed and
   revoked with maker and expiry. Covers the sixth and seventh.
3. **Interface:** the tab Rights in nav1, the rights dialog of documents, folders and files, owner and who sees in the
   facts panel, commenting without editing on a block, the administration in the profile with users and groups, and
   taking back an administrator. Covers the third, eighth and ninth.
4. **The log** of access and changes, shown in the tab Rights. Covers the last.

Later, with the Samba part at the end: Samba users and rights, the limit of a share (ADR-29), the domain (the tenth and
eleventh criterion). Passwords in the text (ADR-7) are a piece of their own; their attack round follows once they exist,
while the attack round on the rights runs after step 4.

**Built on 2026-11-19:** steps 1 to 4 ([docs/rights.md](../docs/rights.md)): roles, inheritance, ownership, groups, the
one check, share links, the tab Rights with the rights dialog and the access check, administrators in the profile and
taking them back, and the log. Open: the Samba part (users, rights and the limit of a share, the domain), and passwords in
the text (ADR-7) with their attack round. The retention of the log (365 days per tenant, set from 30 to 3650) and new
collections that belong to their maker alone followed the same day (ADR-29).

**Settled on 2026-11-20:** [ADR-30](ADR-30-samba-share.md) says how the Samba part is built: a companion container
per tenant with smbd and a helper that takes a few defined requests over a Unix socket, so UnderDoc never runs as root;
an app password per account (its NT hash sealed in the vault); POSIX ACLs per folder computed from the roles; a folder in
which someone holds a role without reading the folder above becomes a share of its own for them. The domain stays later.

**Built on 2026-11-20 (ADR-30, [docs/samba.md](../docs/samba.md)):** Samba users with an app password per account (NT
hash sealed in the vault), managed through the companion without root; roles as POSIX ACLs per folder, the limit of a
share (rights per folder, single files cannot break the inheritance in a shared library, a folder someone reads without
the folder above becomes a share of its own), administrators list and remove Samba users. Open: the domain, and passwords
in the text (ADR-7) with their attack round.
