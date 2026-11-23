-- SPDX-License-Identifier: MIT
-- The rules page is a setting of the administrators (B-5). Until now it was the oldest page of the kind agent notes
-- tagged rules, so an agent after a prompt injection could tag an older page, or rewrite the page, and every agent read
-- its rules. Now an administrator names the page in the browser; agents read it and do nothing else with it or below it,
-- whatever the entries say, and set or take away the tag rules nowhere. Where a tenant had a rules page by the tag, it
-- stays its rules page.

CREATE TABLE agent_settings (
    tenant     text NOT NULL DEFAULT current_setting('underdoc.tenant', true) REFERENCES tenants (id),
    rules_page text,
    changed    timestamptz NOT NULL DEFAULT now(),
    changed_by text NOT NULL DEFAULT '',
    PRIMARY KEY (tenant),
    FOREIGN KEY (tenant, rules_page) REFERENCES objects (tenant, id) ON DELETE SET NULL (rules_page)
);

ALTER TABLE objects NO FORCE ROW LEVEL SECURITY;
INSERT INTO agent_settings (tenant, rules_page)
SELECT DISTINCT ON (tenant) tenant, id FROM objects
WHERE kind = 'page' AND page_kind = 'agent-notes' AND 'rules' = ANY (tags) AND deleted_at IS NULL
ORDER BY tenant, created, id;
ALTER TABLE objects FORCE ROW LEVEL SECURITY;

ALTER TABLE agent_settings ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_settings FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_only ON agent_settings
    USING (tenant = current_setting('underdoc.tenant', true)) WITH CHECK (tenant = current_setting('underdoc.tenant', true));

-- The decision of ADR-36 (migrations 030, 035, 037), with the rules page closed to agents but for reading (B-5).
CREATE OR REPLACE FUNCTION object_decide(obj text, viewer text, act text, agent boolean, with_links boolean) RETURNS right_decision
LANGUAGE plpgsql STABLE AS $$
DECLARE
    cur        text := obj;
    level      record;
    e          record;
    inheriting boolean := true;
    is_self    boolean := true;
    in_private boolean := false;
    decided    boolean := false;
    d          right_decision := ROW(false, 'none', '', '', '', '')::right_decision;
    ceiling    right_decision;
    gs         text[];
    gd         text[];
    ga         text[];
    gad        text[];
    svc        boolean;
    lid        text;
    limited    text[];
    archive    text;
    rules      text;
    rules_page text;
BEGIN
    IF viewer IS NULL OR viewer = '' OR act IS NULL
            OR act NOT IN ('read', 'comment', 'create', 'write', 'delete', 'share', 'manage')
            OR NOT EXISTS (SELECT 1 FROM accounts WHERE id = viewer) THEN
        RETURN ROW(false, 'unknown', '', '', '', '')::right_decision;
    END IF;
    gs := account_groups(viewer);
    gd := account_direct_groups(viewer);
    ga := account_allow_groups(viewer);
    gad := account_allow_direct_groups(viewer);
    svc := account_service(viewer);
    agent := coalesce(agent, false) OR svc;
    IF agent THEN
        SELECT s.rules_page INTO rules_page FROM agent_settings s;
    END IF;
    WHILE cur IS NOT NULL LOOP
        SELECT id, parent, owner, private, inherit, archived INTO level FROM objects WHERE id = cur;
        IF NOT FOUND THEN
            EXIT;
        END IF;
        IF level.private THEN
            IF level.owner <> viewer THEN
                RETURN ROW(false, 'private', level.id, 'person', level.owner, '')::right_decision;
            END IF;
            in_private := true;
        END IF;
        IF level.parent IS NULL AND level.archived IS NOT NULL THEN
            archive := level.id;
        END IF;
        IF rules_page IS NOT NULL AND level.id = rules_page THEN
            rules := level.id;
        END IF;
        IF agent AND ceiling IS NULL THEN
            IF EXISTS (SELECT 1 FROM grants g WHERE g.object = cur AND g.kind = 'agents' AND g.action = act) THEN
                ceiling := ROW(false, 'agents', cur, 'agents', '', 'deny')::right_decision;
            END IF;
        END IF;
        IF inheriting AND NOT decided THEN
            IF is_self AND level.owner = viewer THEN
                d := ROW(true, 'owner', cur, 'person', viewer, 'allow')::right_decision;
                decided := true;
            ELSE
                SELECT g.kind, g.who, g.value INTO e FROM grants g
                WHERE g.object = cur AND g.action = act AND g.kind <> 'agents'
                  AND ((g.kind = 'tenant' AND (g.value = 'deny' OR NOT svc))
                       OR (g.kind = 'person' AND g.who = viewer)
                       OR (g.kind = 'group' AND g.value = 'deny' AND g.who = ANY (CASE WHEN g.subgroups THEN gs ELSE gd END))
                       OR (g.kind = 'group' AND g.value = 'allow' AND g.who = ANY (CASE WHEN g.subgroups THEN ga ELSE gad END)))
                ORDER BY g.value = 'deny' DESC, CASE g.kind WHEN 'person' THEN 0 WHEN 'group' THEN 1 ELSE 2 END, g.who
                LIMIT 1;
                IF FOUND THEN
                    d := ROW(e.value = 'allow', 'entry', cur, e.kind, e.who, e.value)::right_decision;
                    decided := true;
                ELSIF level.owner = viewer THEN
                    d := ROW(true, 'owner above', cur, 'person', viewer, 'allow')::right_decision;
                    decided := true;
                ELSIF with_links THEN
                    -- A link lends the actions of its mode, as far as its maker still may share it and still holds
                    -- the action (T-34).
                    SELECT l.id INTO lid FROM share_links l JOIN share_link_users u ON u.link = l.id
                    WHERE l.object = cur AND u.account = viewer AND l.mode IN ('comment', 'edit') AND act = ANY (preset_actions(l.mode))
                      AND (l.expires IS NULL OR l.expires > now())
                      AND (object_decide(l.object, l.created_by, 'share', false, false)).allowed
                      AND (object_decide(l.object, l.created_by, act, false, false)).allowed
                    ORDER BY l.id LIMIT 1;
                    IF FOUND THEN
                        d := ROW(true, 'link', cur, 'link', lid, 'allow')::right_decision;
                        decided := true;
                    END IF;
                END IF;
            END IF;
            IF NOT level.inherit THEN
                inheriting := false;
            END IF;
        END IF;
        is_self := false;
        cur := level.parent;
    END LOOP;
    -- An archived space is read only for everyone, its owner and administrators included, until it is restored (T-34).
    -- Manage stays, so who may read it can still be set.
    IF archive IS NOT NULL AND act IN ('comment', 'create', 'write', 'delete', 'share') THEN
        RETURN ROW(false, 'archived', archive, '', '', 'deny')::right_decision;
    END IF;
    IF in_private THEN
        d := ROW(true, 'private', obj, 'person', viewer, 'allow')::right_decision;
    END IF;
    IF agent AND d.allowed THEN
        IF act IN ('share', 'manage') THEN
            RETURN ROW(false, 'agents default', '', 'agents', '', 'deny')::right_decision;
        END IF;
        -- The rules page: agents read it, nothing else, and nothing below it (B-5).
        IF act <> 'read' AND rules IS NOT NULL THEN
            RETURN ROW(false, 'rules page', rules, 'agents', '', 'deny')::right_decision;
        END IF;
        IF ceiling IS NOT NULL THEN
            RETURN ceiling;
        END IF;
        SELECT actions INTO limited FROM agent_limits WHERE account = viewer;
        IF FOUND AND NOT act = ANY (limited) THEN
            RETURN ROW(false, 'agents own', '', 'agents', viewer, 'deny')::right_decision;
        END IF;
    END IF;
    RETURN d;
END
$$;
