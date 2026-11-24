---
type: Bug
title: 'Review of B-7: move cycles, reset as a back door, read snapshots that do not scale, gate key break'
status: offen
points: 13
id: B-8
milestone: M-30
created: '2026-11-24'
---
# B-8 — Review of B-7: move cycles, reset as a back door, read snapshots that do not scale, gate key break

Review of #189 and #190 (B-7) by the team wiki, 2026-11-24. The high findings of B-6, T-33 and T-38 are fixed. First after the pause, before T-40. Decisions by the master in each point.

**Medium**
1. **Move cycles:** only the object and the target are locked, the way up is read without a lock; two moves with disjoint locks (A to C, D to A') build a cycle without a space. Fix: on a change of parent, pg_advisory_xact_lock per tenant, as lockGroups does for group_groups. Test with two parallel moves.
2. **Reset as a back door:** apiReset ignores how; a group admin with the approver role gets a fresh sign-in link through reset and wait-approval, what B-7 forbids for the link. onlyMine counts only members of local groups, not synced groups, group admins of other groups or direct entries on spaces and collections. Fix: reset only as administrator, delegate or inviter, as for the link; onlyMine also counts other group admins, synced groups and direct entries.
3. **Read snapshots do not scale** (performance, high with many accounts): every read stores all reading accounts (object_decide per account), the compare runs @> over every row of the window. Decision: per read store everyone (bool), the count and a hash of the sorted list; the full list only when not everyone, deduplicated by hash; compare by anti-join. Measure with EXPLAIN ANALYZE at 2000 and more accounts.
4. **The pair check of copies** costs pairs times accounts times two decide calls under locks. Narrow the accounts first (readers of the target parent plus accounts with entries in the original tree) and set a limit for copies by agents. Measure.
5. **Gate key break (operations):** TinySesam's break (T-111, T-72) is on its main, a release is announced; then UnderDoc's automation key and all open sign-in links are void once, the gate answers 401, privileged stays unknown, delegates and group admins can only lock, purge and emptying the trash fail. Fix: update note in docs/sign-in.md and docs/accounts.md (new key, replace the key file, issue invitations again); tell a 401 from a network failure and say so clearly.

**Low**
6. With lock and an unknown gate state a delegate locks an admin promoted only in the gate; ErrNotFound counts as known and not privileged. Delegates lock only accounts without any admin hint while the gate state is unknown.
7. Deleting a space takes FOR UPDATE and can deadlock with a write (40P01); deleteObject and lockTree use FOR NO KEY UPDATE, FOR UPDATE only when archiving.
8. RemoveFromMyGroups also hits archived subgroups: AND g.archived IS NULL.
9. Accounts created or unlocked after a read make every later write wider, also everyone to everyone: everyone on both sides counts as equal, accounts created after the read are ignored.
10. Checking off resets the reads of the person who checks, not of the person behind the agent; review is guarded only by noAgent. Delete only when the author of the head version is the viewer, and require a browser session.
11. Escaping of meta lines is not lossless: a line starting with a backslash and a meta prefix changes on read and write back. Always add one backslash on read, remove exactly one on write.
12. The cutoff for emptying the trash is bound only by the count: sign a hash of the sorted ids and compare with what is taken.

**Notes:** object_public compares yes or no, not which link (a copy under another link reaches new link holders; document or compare link sets); invited_by stays valid while the inviter admins any group (optional: bind to the group, expire on foreign membership); migration 042 leaves invited_by empty for old invitations (stricter, fine); mayOpenUp counts objects in the trash, so an invisible deny there blocks opening up to 30 days: say why in the error; reads without a foreign key survive purge: a maintenance job after the window.
