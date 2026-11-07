---
id: M-5
type: Milestone
title: Permissions and sharing
status: offen
tags: [permissions, security]
created: 2026-11-06
---

Permissions are good enough for company data.

**Done when this is true:**

- Every document has an owner, editors, commenters and viewers, per person or group, and ownership can be transferred.
- Parents pass default roles on to what is below them, as T-9 decides, and embedding bypasses nothing.
- Commenters can comment on a document, without an editor.
- Notes are private per user.
- Public shares use a token, list all embedded objects for release and only show frozen data.
- Share links can be listed and revoked, and each one shows who created it and when it expires.
- The metadata adds the owner and who can see a document.
- An administrator role sees the admin panel: modules, users and groups, and the rest of the system settings (ADR-16).
- The app manages Samba users and permissions without running as root, with a password per user. Roles reach the share as
  far as T-1 has proven them.
- Samba can optionally join a domain.
- Passwords in text work as described in ADR-7, and an attack round has looked at them.
- Access and changes are logged.
