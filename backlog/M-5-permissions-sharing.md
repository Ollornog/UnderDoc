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

- Permissions are inherited from library to folder to page, and embedding bypasses nothing.
- Notes are private per user.
- Public shares use a token, list all embedded objects for release and only show frozen data.
- The app manages Samba users and permissions without running as root, with a password per user.
- Samba can optionally join a domain.
- Passwords in text work as described in ADR-7, and an attack round has looked at them.
- Access and changes are logged.
