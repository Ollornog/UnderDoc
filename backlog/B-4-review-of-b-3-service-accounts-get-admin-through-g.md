---
type: Bug
title: 'Review of B-3: service accounts get admin through gate roles, archive bypass, facet leaks'
status: offen
id: B-4
milestone: M-30
created: '2026-11-23'
---
# B-4 — Review of B-3: service accounts get admin through gate roles, archive bypass, facet leaks

Review of B-3 (PR #168) by the team wiki, 2026-11-23. Fix all of these before MCP (T-21) is merged, because MCP opens new doors.

1. **Medium:** an allowed service account still gets admin and spaces rights from gate roles. Identify sets Admin and Spaces from the roles also for kind service; maySpace, the admin API and adminOnly do not check for a service account. Fix: for a service account, Admin and Spaces are always false in Identify. Test with Remote-Groups carrying the admin and the spaces role.
2. **Low:** the rule that a group's own admins cannot archive it can be bypassed: someone who manages G and K puts G under K, archives G as admin of the parent, and takes G out again. Fix: admins of G never archive G, even when they also admin a parent.
3. **Low:** search facets count hits a service account may not see. Everyone, inherited and archived groups drop out of the reader tokens, so their deny tokens do not apply either; MayFind checks only the hits, the facets still count them. Fix: count facets from checked hits, or query denies separately.
4. **Low:** the same facet leak for people in archived groups (GroupsOf includes archived groups).
5. **Low:** in legacy mode a service account without Remote-Kind becomes a person. ADR-32 says the refusal holds in legacy mode too. Fix: limit the doc to "only when the proxy sends the kind" and add a start warning.
6. **Low:** HeldBy lists allows through archived groups and tenant allows for service accounts as valid. Use the allow groups, or mark these entries as not in effect.
7. **Note:** archived groups can still be set as readers of passwords (no check on archived). No effect, but contradicts the docs. Same check as when setting entries.
8. **Note for T-33:** the approver runs before the hold check in purge, so a one-time approval is used up even when the purge is refused; emptying the trash would need one approval per item. Check first, then approve, and plan one approval for emptying the trash.
9. **Note:** ADR-36 says a service account gets allow only from local groups it is put in directly, but the allow groups go up to local parent groups. Align the ADR and add a test.

Missing tests: for 1, 2, 3 and 5, and purge with a wrong name (400) and without Sec-Fetch-Site or Origin (403) through the API, unless already covered.
