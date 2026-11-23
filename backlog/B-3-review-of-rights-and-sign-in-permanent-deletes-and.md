---
type: Bug
title: 'Review of rights and sign-in: permanent deletes and service accounts'
status: erledigt
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

**Built on 2026-11-23** (migration 035; [docs/rights.md](../docs/rights.md#archived-groups),
[docs/sign-in.md](../docs/sign-in.md#the-gate); ADR-32 part 3, ADR-36 part 2). Decided in the build:

- **Groups:** `DELETE /api/groups/<id>` archives, `POST /api/groups/<id>/restore` brings back. Nothing deletes a group
  for good any more, so no such action is left for administrators either. Archived, allow entries hold for nobody and
  deny entries keep holding (fail closed, reasons in ADR-36). Admins of an archived group manage nothing through it.
- **Purge:** owner or administrator, browser session, name typed in and checked by the server; emptying the trash needs
  `Trash` typed in and takes only what the person may delete for good. Whoever only deleted it no longer purges it. The
  hook for the gate's signed approval is `db.SetApprover` (action `underdoc.purge`), unset until T-33. The trash still
  empties itself after 30 days.
- **Spaces:** `UNDERDOC_SPACES_ROLE` (`underdoc-spaces`) or an administrator for every way a collection reaches the top:
  made, made among Files, copied or moved there. Without a request (the command line, an import) the operator acts.
- **Service accounts:** the allowlist is the setting `UNDERDOC_SERVICE_ACCOUNTS` rather than a table with a command,
  since the fleet sets a setting once and idempotently. An allowed ID is a service account whatever kind is sent, always
  an agent, and never in the audience of the tenant or of a role of the gate.
- **Remote-Kind:** the gate announced the header on 2026-11-23, so the fill-in is gone from the default; a missing or
  unknown kind is 403, and `UNDERDOC_GATE_KIND=legacy` is the only way back.
- The attack round of T-26 for these doors is in `internal/db/review_b3_test.go`, `internal/web/review_b3_test.go`, the
  browser tests of the trash and the tab Rights, and every guard is proven by a mutation.
