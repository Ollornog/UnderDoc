---
type: Task
title: 'Accounts through the gate''s account API: invite, lock, new link, delegated admin'
status: offen
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
