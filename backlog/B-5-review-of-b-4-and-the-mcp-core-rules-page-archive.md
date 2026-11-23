---
type: Bug
title: 'Review of B-4 and the MCP core: rules page, archive bypass, facet regression, agent moves'
status: offen
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
