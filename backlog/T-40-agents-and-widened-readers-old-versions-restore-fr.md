---
type: Task
title: 'Agents and widened readers: old versions, restore from trash, re-uploaded files'
status: offen
points: 13
id: T-40
milestone: M-30
created: '2026-11-24'
---
# T-40 — Agents and widened readers: old versions, restore from trash, re-uploaded files

Left open by the attack round T-38 (2026-11-24) because they need a design decision. Proposals revised after the review by the team wiki on 2026-11-24; the maintainer may change them before this is built.

- **Old versions (path 4):** after content was removed from a restricted page and the page was then opened or moved to wider readers, old versions show the removed content to the new readers. A history per person would need a log of rights per account. Proposal: a **horizon per object**: every event that widens the readers sets a time stamp; versions older than the horizon are visible only to owner and managers. The person who widens is asked "give the history along?", default no.
- **Restore from trash (path 5):** restoring to a parent that has more readers since the delete. Proposal: restore only to the original parent; if it is gone, the object comes back narrow (private with its owner). For agents also agentNoWider on restore.
- **Download and upload again (path 11):** no refusal by sha256, because one changed byte or another format gets around it, and a refusal is an oracle ("this content exists where you cannot read"). Proposal: compare only with blobs the agent read in its own read window, and **mark instead of refuse**, as in T-38.
- The read window itself (one working day per person, no limit, snapshot at read time, reset by the person) is fixed in B-7.
- Test for the data part of migration 039 (a private rules page taken over from the tag is dropped), missing since B-6.
- **From B-7 (finding 13), decided by the master:** the network part of VerifyApproval (signature, JWKS) runs outside any transaction; storing the jti (unique) and the delete run in one transaction, so the approval is spent exactly when the delete commits. This keeps the rule of B-6 (no outside call inside an open transaction) and closes the gap where a failed delete spends the approval.
