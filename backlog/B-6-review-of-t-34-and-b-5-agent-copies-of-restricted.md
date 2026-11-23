---
type: Bug
title: 'Review of T-34 and B-5: agent copies of restricted trees, public links, link rights on the object'
status: erledigt
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

**Built on 2026-11-23** (migration 039; [docs/rights.md](../docs/rights.md#known-limits),
[docs/mcp.md](../docs/mcp.md#the-rules-page)). Each finding, as built:

1. `agentNoWider` takes every object a copy makes (the tree read before the copy), so a copy below its top is checked
   against the readers of the target like its top. Move checks its top, as decided.
2. `object_decide` (migration 039) asks the maker of a link for share and the action on the object asked for, `obj`,
   not on the object of the link.
3. `object_public` (migration 039): a live link that shows its object (every mode but upload) on the target or above it.
   A target that is public takes from an agent only what is public already. Read, comment and edit links all show their
   object at `/s/…`, so all three count, not only read.
4. `redirectAll` leaves out pages in an archived space and the rules page; their redirect leads on.
5. `SetEntries` locks the object (`FOR UPDATE`) and reads the old entries and decides `mayOpenUp` in the transaction
   that writes them.
6. `mayOpenUp` asks every action opened up on the object and every object below it that inherits and is not private,
   as `mayTakeAll` does for deleting.
7. `holdSpace` takes the row of the space `FOR SHARE` and refuses an archived one, in the one place each write of
   content passes: `db.change` (blocks), `addVersion`, `addAttachment` and `insertObject` (which opens a transaction of
   its own where its caller has none). Archiving updates that row, so it waits for such a write.
8. For a move by an agent, a link (not upload) on what moves whose maker may not share it where it lies but may share
   the target counts as making it public.
9. Kept as decided, with `TestServiceCountsWithAnAllowAndADenyOnOneLevel`: removing `request.Service` turns it red.
10. `EmptyTrashBefore(cutoff)`: candidates first, then one transaction locks those deleted at or before the cutoff,
    checks each (`mayTakeAll`), asks the approver once with the cutoff as its subject (`db.Approver.Approve(ctx, action,
    subject)`), and deletes them in one statement; nothing but the deletion follows the approval. `Purge` locks its
    object before its check the same way. T-33 wires the gate's VerifyApproval onto this order and the cutoff.
11. Migration 039 drops a rules page taken over from the tag when it or anything above it is private, and marks the
    others `from_tag`; `SetRulesPage` refuses private and archived pages and clears the mark; administrators see it in
    the settings of the page (*Confirm as the rules page*) and in `GET /api/rules-page`.
12. `context` gives the rules page as written (raw Markdown, up to 16 KB), embeds not followed, and says so.
13. `ExpireTrash` leaves out what lies in an archived space; restoring a space moves `deleted_at` of its trash on by the
    time it was archived.
14. `blocks` of a page of the database is always sent, `[]` when empty (`null` for the folder source), so the workspace
    offers the kind of an empty page.

Limits documented in [docs/rights.md](../docs/rights.md#known-limits). Attack round in `internal/db/review_b6_test.go`
and `internal/web/review_b6_test.go`; every guard proven by a mutation (24, all red). Not covered by a test: the data
part of migration 039 (dropping a private rules page taken over by 038), which runs once on existing data.
