---
type: Bug
title: 'Review of T-34 and B-5: agent copies of restricted trees, public links, link rights on the object'
status: offen
id: B-6
milestone: M-30
created: '2026-11-23'
---
# B-6 — Review of T-34 and B-5: agent copies of restricted trees, public links, link rights on the object

Review of #174 (T-34) and #175 (B-5) by the team wiki, 2026-11-23. Decisions by the master in each point.

**High**
1. **Agent copy of a tree with a restricted child** (copy, agentNoWider checks only the top object). Copy keeps only the tenant allow for children, not person or group entries, denies or a break of inheritance. Folder O readable for the team, page K inside with broken inheritance only for Anna; Anna's agent copies O into another team collection and K becomes readable for the team. Decision: agentNoWider runs for every copied object. Negative test. Move keeps the children's entries, the top check suffices there.

**Medium**
2. **Link rights checked on the ancestor, not the object** (migration 037, kept in 038). Carl has manage on space S, makes an edit link on S, then gets a person deny delete on page P; Dora deletes P through the link. Fix: object_decide on the requested object for the link's creator. Negative test with a link on the parent.
3. **Public read links count as no readers** (agents.go). A folder with a public link serves live; an agent moves a file only Anna could read into it and it is public. Decision: an active read link on the target or an ancestor counts as unlimited readers. Test.

**Low**
4. Moving a block rewrites ref= in all referring pages without a check, also in archived pages and in the rules page, with the mover as author. Skip archived pages and the rules page; the redirect is enough.
5. entriesOf and mayOpenUp run outside the transaction, setEntries then replaces all (a deny set at the same time is overwritten). Read in the transaction and lock.
6. mayOpenUp checks only the object; a manager with a deny on child K can give delete on O to others who then delete K. Decision: check the inheriting descendants, as mayTakeAll does.
7. setArchived takes no lock; check-then-write paths such as finishing an upload land in the archive. Decision: FOR SHARE on the space row in write transactions, in one central place.
8. A link of the moved object that was dead at the old place (creator without share) becomes active at the target. Count these links for agent moves.
9. The mutation called equivalent in B-5 is not: with an allow for a service account and a tenant deny on the same level, facets count the denied document without request.Service. Keep the term and add this test.
10. EmptyTrash takes the approval before mayTakeAll and binds it to an empty subject, so what arrives between confirming and the request is deleted too. Decision: check everything, lock, approve, delete; bind the approval to a cutoff (deleted_at at or before it). The approval call never runs inside an open transaction that can still fail afterwards.
11. Migration 038 takes the oldest tagged page tenant-wide, also private ones. Exclude private pages, show a taken-over page as "taken over from the tag, please confirm" to admins. SetRulesPage refuses private and archived pages.
12. If the rules page embeds blocks from pages agents can write, agents change the rules indirectly. Decision: context gives the rules page without resolving embeds.
13. Trash of an archived space expires after 30 days but cannot be restored while archived. Decision: expiry pauses while archived.
14. Existing bug: empty pages send no blocks to the front end, so the page kind choice is missing.

**Document as limits** (no code): facets may trail a new deny briefly; Members holders write in the browser but see Samba read-only; import sets archived without the fixed root check (CLI only, the CHECK protects); group admins pass rights they do not hold through membership (the rule covers entries); copy keeps tags, so an agent can make a page tagged rules (no effect since B-5); "agents publish nothing" covers move, copy and block moves, not reading and writing anew.
