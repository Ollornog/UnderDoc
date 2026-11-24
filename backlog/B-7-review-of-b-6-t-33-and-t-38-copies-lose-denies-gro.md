---
type: Bug
title: 'Review of B-6, T-33 and T-38: copies lose denies, group admins manage any account, read window'
status: offen
points: 13
id: B-7
milestone: M-30
created: '2026-11-24'
---
# B-7 — Review of B-6, T-33 and T-38: copies lose denies, group admins manage any account, read window

Review of #178 (B-6), #182 (T-33) and #186 (T-38) by the team wiki, 2026-11-24. Must be fixed before any productive use. Decisions by the master in each point.

**High**
1. **A copy keeps the tenant allow but loses person and group denies** (copy, agentNoWider checks only target readers within source readers). A page readable for the tenant with a deny for X, copied by an agent into a target only for group G, becomes readable for X. Fix: inside the copy transaction, for each pair (original, copy): if any account may read the copy but not the original, roll back with ErrPublish. Closes the check-then-act gap too.
2. **A group admin manages any account** (MayManagePerson with SetMember): the admin of one group adds an account by id, then makes a new sign-in link and signs in as that person, or locks them everywhere. Decision: group admins manage only people who are only in their groups or whom they invited (store the inviter); new sign-in links only for the inviter, admins and delegates; a lock by a group admin means only removing from the group.

**Medium**
3. holdSpace is missing in the main write path (WriteBlocks for MCP write and replace), in take back, move (old and new space), delete and rename; a parallel archive lets the commit land in the archive. Add holdSpace after the FOR UPDATE on the object, everywhere; check the FOR SHARE then UPDATE order for a deadlock with two parallel uploads at the top level.
4. The read window compares only the last 50 reads, search hits count too: read P, three searches, P is out. Decision: no limit, one EXISTS in SQL; store the narrowness (readers snapshot) when reading; the window is one working day per person (setting, default 12 h) instead of 2 h; the person resets it in the browser when checking off the mark.
5. Reads vanish with the trash (deleted_at IS NULL and cascade): read, trash, write publicly, no mark. Keep reads independent of the trash, with the snapshot of 4.
6. read_file_page records no read, set_front_matter has no check before writing. Add both. REST, WebDAV and Samba with keys are not covered: document that the mark covers MCP only.

**Low**
7. Dead links that wake up on a move are checked only with mode read; count every mode except upload.
8. A rules page taken over from the tag and not confirmed is still served to agents and inlined in context: NotFound until an admin confirms.
9. privileged comes from the groups stored at the last sign-in; accounts from add or adopt and freshly promoted people look unprivileged. Check roles through the gate client.
10. The cutoff for emptying the trash comes from the app clock, deleted_at from the database; take now() from the database and stop when more items match than were signed.
11. "written by others" looks only at the last author; mark as soon as any version comes from someone else.
12. No unforgeable border between meta and content in MCP answers: page text can fake "readers: only you" or the end of the rules page. Use a random border per answer and escape meta prefixes inside content.
13. The approval runs inside the open transaction under FOR UPDATE locks; if DELETE or commit fail afterwards, the jti is spent. Order: check and lock, commit the lock state, approve, delete.
14. mayOpenUp skips deleted descendants; a child restored later inherits the opening despite its deny. SetInherit calls mayOpenUp without a lock.
15. ReadersOf runs object_decide per account, up to 51 times per write; use EXISTS with early exit.

**Notes to document or small:** lock locally first, then in the gate; the door cache of other instances lets a locked account in for up to 5 minutes (rights apply at once); open SMB sessions after a lock; check the delegate role against the spaces role at start; marks fail open on errors (deliberate). Raise the contract test to TinySesam 0.30.x (the last version with migrations; the team wiki runs reset_access and VerifyApproval on 0.30.0).
