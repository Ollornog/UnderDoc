---
id: M-22
type: Milestone
title: Mail in the workspace
status: offen
blocked_by: [M-5]
tags: [module, people]
created: 2026-11-07
---

Mail appears next to the documents it belongs to, as a module an administrator can switch off. Only an idea so far: the talk before
this milestone (ADR-14) settles the scope.

**To settle in that talk:**

- Reading only, or also sending and replying.
- One account per user over IMAP and SMTP, or a shared mailbox, and where the credentials live (ADR-7).
- Whether a mail can be linked to a document or stored as one, and what that means for roles, versions and deletion.

**Done when this is true:**

- Nav1 shows Mail, nav3 lists the mail, and a mail opens in a tab like a document.
- A user only ever sees their own mail, and mail never reaches search or an AI without being asked to.
- Switched off, the module leaves no entry behind.
