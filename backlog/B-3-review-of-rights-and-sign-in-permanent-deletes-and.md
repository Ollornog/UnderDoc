---
type: Bug
title: 'Review of rights and sign-in: permanent deletes and service accounts'
status: offen
id: B-3
milestone: M-30
created: '2026-11-23'
---
# B-3 — Review of rights and sign-in: permanent deletes and service accounts

Found by the team wiki's review of M-30 (main 45baebf) against its own code, 2026-11-23.

- **Deleting a group is permanent, and a group admin may delete their own group** (DeleteGroup). Deleting a group archives it instead, so it can be restored. Only an administrator of a group above it, or an UnderDoc administrator, may do it, and only while it has no active subgroups.
- **Purging from the trash is open to any session with delete on the subtree** (trash.go). ADR-35 keeps such actions with administrators in the browser until the gate offers approvals. Purge needs: an UnderDoc administrator or the owner, a browser session (Remote-Auth session), and the name typed in. Once the gate's VerifyApproval is wired (T-33), it also needs a signed one-time approval for the app action, with its jti used once.
- **Anyone may create a top-level space** (CreateCollection), although ADR-35 reads a gate role for it. Read a configurable role such as underdoc-spaces; administrators keep it.
- **Service accounts (Remote-Kind service) get an account of their own and count as signed in.** Refuse a service account unless an administrator has allowed its Remote-Id. It then counts as an agent and never belongs to the audience everyone signed in (tenant).
- A missing Remote-Kind counts as user only until the gate announces that the header always arrives. Then remove that fill-in and set UNDERDOC_GATE_KIND=required together (ADR-32, TinySesam 2026-11-23).

Every fix is proven by a mutation, and the attack round of T-26 is repeated for the changed doors.
