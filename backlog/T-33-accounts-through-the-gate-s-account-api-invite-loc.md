---
type: Task
title: 'Accounts through the gate''s account API: invite, lock, new link, delegated admin'
status: erledigt
points: 8
id: T-33
milestone: M-30
created: '2026-11-23'
---
# T-33 — Accounts through the gate's account API: invite, lock, new link, delegated admin

ADR-32 removed the built-in invitations, and the account API of ADR-35 is not built yet, so nobody can invite. The team wiki needs this before it moves.

- **Invite:** a group admin invites a colleague from the group's page. UnderDoc checks itself that the person who clicked is an admin of that group, calls the gate's account API with its service account and TinySesam-On-Behalf-Of, and uses only roles on its allowlist. The profile is bound at once to the new Remote-Id, and the person is in the group straight away.
- **New sign-in link** (only before the first sign-in), **lock and unlock** in the gate, and a **local lock flag** on the profile that UnderDoc enforces on its own as well.
- **Reset access** as the gate's approval tinysesam.account.reset_access; the approver role is never on the allowlist.
- All of these only in a browser session, never for agents.
- **Existing accounts:** adopt them through the account API.
- **Command line for imports:** underdoc account add <remote-id> <login> <name> creates a bound profile without an admin mark. Profiles without a Remote-Id are never bound by name.
- **Two admin levels** as roles of the app in the gate: the UnderDoc administrator, and a delegated administrator with a role of its own (ADR-35).
- Wire VerifyApproval of the gate's Go client (TinySesam 0.29.0) for B-3's permanent deletes. From B-4 (finding 8):
  the approver runs before the hold check in a purge today, so a one-time approval would be used up even when the purge
  is refused, and emptying the trash would need one approval per item. Check first, then approve, and plan one approval
  for emptying the trash.

Built against the gate's Go client pinned by commit.

**Built on 2026-11-24** ([docs/accounts.md](../docs/accounts.md), migration 040; ADR-35 "Built on"). As built:

- Package `gateapi`: the account API (provisioning, invite, read, list, lock, unlock, new link) and the approvals (ask,
  pick up) with the key from `UNDERDOC_GATE_SERVICE_KEY_FILE`; `gatetest` is a stand-in for the tests of the web, and a
  contract test (`-tags gate`) runs the client and UnderDoc's verifier against a real TinySesam (`scripts/gate/gate.py` in
  the published image `v0.29.1`, pinned by digest, `scripts/check-services.sh`).
- Invite (`POST /api/groups/<id>/invite`), account (`GET /api/accounts/<id>`), new link, lock and unlock
  (`PUT /api/accounts/<id>/lock`), reset (`POST /api/accounts/<id>/reset`, `POST /api/approvals/<id>/wait`), in the tab
  Rights. Group admins for the persons in their groups, the delegated administrator for every account but an
  administrator's, administrators for every account; only a person in the browser.
- The lock of UnderDoc (migration 040): refused at the door (web, API, MCP), in `object_decide` (so links of the account
  and links it took), and on the share.
- Two levels as app roles, decided in `account.User.Has`: the delegated administrator archives groups (B-5) and manages
  accounts.
- VerifyApproval for purge and emptying the trash, on the order and the cutoff B-6 built; one approval for emptying.
- `underdoc account add` and `underdoc account adopt`.
- Every step in the log with its On-Behalf-Of.

Not covered: the gate in front of the fleet has no `apps` in its ready-made gateway yet (TinySesam T-97), so the
function stays off there until the gate offers it.
