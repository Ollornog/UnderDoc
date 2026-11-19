-- SPDX-License-Identifier: MIT
-- The group of writers of the folder of the tenant keeps edit in its narrower folders (ADR-29, 2026-11-19). Under ADR-20
-- the writers edited everywhere below the folder of the tenant, whatever level a folder set. Migration 022 turned a
-- narrower level into a break of the inheritance with a grant to the tenant, and copied nothing else onto it, so the
-- writers only read there. This grants them edit again on each such break.
--
-- A break made by 022 is one whose grant to the tenant has no granter and is narrower than edit; the grant of the
-- writers made by 022 on the folder of the tenant has no granter either. Breaks and grants someone made since carry who
-- made them and stay as they are.

ALTER TABLE objects NO FORCE ROW LEVEL SECURITY;
ALTER TABLE file_roots NO FORCE ROW LEVEL SECURITY;
ALTER TABLE grants NO FORCE ROW LEVEL SECURITY;
ALTER TABLE groups NO FORCE ROW LEVEL SECURITY;
-- Who may read changes for no one (the writers read there through the tenant already), so the search need not hear of it.
ALTER TABLE grants DISABLE TRIGGER grants_to_search;

WITH RECURSIVE below AS (
    SELECT r.tenant, r.object AS root, o.id FROM file_roots r JOIN objects o ON o.tenant = r.tenant AND o.parent = r.object
    WHERE r.role = 'tenant'
    UNION ALL
    SELECT b.tenant, b.root, o.id FROM objects o JOIN below b ON o.tenant = b.tenant AND o.parent = b.id
)
INSERT INTO grants (tenant, object, kind, who, role)
SELECT DISTINCT b.tenant, b.id, 'group', w.who, 'edit'
FROM below b
JOIN objects o ON o.tenant = b.tenant AND o.id = b.id AND NOT o.inherit
JOIN grants t ON t.tenant = b.tenant AND t.object = b.id AND t.kind = 'tenant' AND t.granted_by = '' AND role_rank(t.role) < 3
JOIN grants w ON w.tenant = b.tenant AND w.object = b.root AND w.kind = 'group' AND w.role = 'edit' AND w.granted_by = ''
JOIN groups g ON g.tenant = w.tenant AND g.id = w.who AND g.source = 'synced'
ON CONFLICT (tenant, object, kind, who) DO NOTHING;

ALTER TABLE grants ENABLE TRIGGER grants_to_search;
ALTER TABLE objects FORCE ROW LEVEL SECURITY;
ALTER TABLE file_roots FORCE ROW LEVEL SECURITY;
ALTER TABLE grants FORCE ROW LEVEL SECURITY;
ALTER TABLE groups FORCE ROW LEVEL SECURITY;
