---
type: Bug
title: 'Review of B-4 and the MCP core: rules page, archive bypass, facet regression, agent moves'
status: erledigt
id: B-5
milestone: M-30
created: '2026-11-23'
---
# B-5 — Review of B-4 and the MCP core: rules page, archive bypass, facet regression, agent moves

Review of #170 (B-4), #171 and #172 (T-21) by the team wiki, 2026-11-23. Fix before MCP is reachable through the gate.

Decisions by the maintainer's session (master), with the reason:

1. **Medium, rules page can be hijacked** (agents.go: oldest agent-notes page with tag rules). An agent after a prompt injection sets the tag rules on an older page, or rewrites the rules page, and every agent gets injected rules. Fix: the rules page is an admin setting (object id); agents may not write the rules page and may not set or remove the tag rules. Test.
2. **Medium, archive bypass still open** (B-4 finding 2): A admins G and H, puts G under H, removes themselves as admin of G, archives G, restores it and adds themselves again. Decision: archiving and restoring a group is for UnderDoc admins only (later also the delegated admin of T-33). Simple and closes every path through nesting. Test for the self-removal path.
3. **Medium, regression in B-4:** facets now count only the 20 to 50 returned hits, so filter values and totals are missing for every reader. Fix: count from the checked hits only when tokens drop out for the reader (service account, or GroupsOf differs from AllowGroupsOf); otherwise take the engine's facets. Test with facets beyond the first page.
4. **Medium, agents publish by moving:** move lets an agent move a private page into a shared collection, which is publishing, against ADR-36. Decision: an agent may not move a page to a place with a wider set of readers. Test private to shared by an agent.
5. **Low:** maySpace refuses only service accounts; an agent with a person's key and the spaces role still creates spaces. Check Agent().
6. **Low:** the redirect answer names targets the caller cannot see (against ADR-29). Name only visible targets.
7. **Low:** move after a bare block (b3) fails because parseRef needs underdoc:<page>#b3. Accept the bare block; test.
8. **Low, shell injection in the upload hint:** the curl line uses %q (double quotes) and file names may contain $( ) and backticks; -T names a local path. Use single quotes and a placeholder <local-file>. Name the link as a bearer URL in docs/mcp.md.
9. **Low:** create_page with text is not atomic; a failed check leaves an empty page and a retry duplicates. One transaction.
10. **Low:** the search total says 97 instead of 100 or more when hits are filtered out; up to 100 single MayFind queries per search (N+1). Say "100 or more", check in one query.
11. **Low:** non-PDF text without page breaks is all page 1, cut at 100 KB. Pseudo pages of about 20 KB.
12. **Low, unsure:** an unchanged read and write with CRLF or edge blank lines counts as a change and marks "written by AI". Normalise; test.
13. **Low, unsure:** the dry run may give wrong LINK warnings for new anchors after the rollback. Test.
14. **Notes:** agents may not trash a top-level collection; context stops at about 50 lines and points to under; purge order (B-4 finding 8: check first, then approve) is fixed now, not only documented, even while SetApprover is unused; /mcp has no rate limit (gate side, note in docs).

**Built on 2026-11-23** (migration 038; [docs/mcp.md](../docs/mcp.md#the-rules-page), [docs/rights.md](../docs/rights.md),
[docs/search.md](../docs/search.md); ADR-34 review notes, ADR-36 part 4). Each finding, as built:

1. The rules page is the setting `agent_settings.rules_page` (`PUT /api/rules-page`, administrators in the browser, a
   button in the settings of a page of agent notes). Migration 038 kept a page tagged `rules` as the setting. The one
   check (`object_decide`) refuses every action but read to agents on the rules page and below it; no agent sets or
   takes away the tag `rules`.
2. Groups are archived and restored by administrators only (`mayArchiveGroup`); tests for the self-removal path.
3. The counts come from the index unless tokens drop out for the reader (a service account, or groups whose allow does
   not hold: `GroupsOf` differs from `AllowGroupsOf`); then from the checked hits. Test with counts beyond the first page.
4. An agent moves or copies nothing, object or block, to where some account may read it that may not read it now
   (`agentNoWider`, `ErrPublish`); a person may. Tests private to shared by move, copy and block move.
5. `maySpace` refuses every agent, and `canSpaces` is false for one; the MCP context says so.
6. `db.Redirect` takes the viewer and leads only to a page the viewer sees, in MCP `read`, embeddings, the problems and
   `/api/redirect`.
7. `move` takes `after` as a bare block (`b3`, `#b3`) for a block.
8. The `curl` line quotes the link in single quotes (`shellQuote`) and names the local file `<local-file>`; docs name
   the link a bearer address.
9. `create_page` with text is `db.CreatePageWithBlocks`, one transaction, the blocks checked first.
10. The check of the hits is one query (`MayFindAll`), and the total says "100 or more" by what the search gave before
    the check (`search.Result.Found`).
11. A text without page breaks above 20 KB is read and indexed in pseudo pages of about 20 KB (`files.Pages`).
12. Writes normalise line ends to LF and drop blank lines at the edges of a block, so an unchanged write back changes
    nothing.
13. The check of a write finds the page itself as written (`ownTarget`), also in a dry run.
14. Agents move no space to the trash; the tree of `context` stops at 50 entries; purge checks first and asks for the
    approval after, emptying the trash asks once (`ApproveEmptyTrash`); `/mcp` has no rate limit of its own, the gate's
    holds (docs).

Attack round in `internal/db/review_b5_test.go`, `internal/web/review_b5_test.go`, the adapted tests of B-3 and B-4 and
the browser test of the rules page; every guard is proven by a mutation.
