-- SPDX-License-Identifier: MIT
-- The review of T-34 and B-5 (B-6).
--
-- A link lends its actions as far as its maker holds them on the object asked for, not on the object of the link
-- (object_decide below): a maker with an edit link on a space and a deny on a page inside it lends no delete there.
--
-- object_public tells whether a live link serves an object to anyone who has it: a link of a mode that shows content
-- (all but upload) on it or above it, not expired, whose maker may still share it. An agent moves nothing there that such
-- a link does not serve already (agentNoWider).
--
-- The rules page migration 038 took over from the tag rules is not a private page, and an administrator confirms it:
-- from_tag marks it until then.

CREATE FUNCTION object_public(obj text) RETURNS boolean LANGUAGE sql STABLE AS $$
    WITH RECURSIVE up AS (
        SELECT id, parent, 0 AS depth FROM objects WHERE id = obj
        UNION ALL SELECT o.id, o.parent, up.depth + 1 FROM objects o JOIN up ON o.id = up.parent WHERE up.depth < 200
    ) SELECT EXISTS (SELECT 1 FROM share_links l JOIN up ON up.id = l.object
        WHERE l.mode <> 'upload' AND (l.expires IS NULL OR l.expires > now())
          AND (object_decide(l.object, l.created_by, 'share', false, false)).allowed)
$$;

ALTER TABLE agent_settings ADD COLUMN from_tag boolean NOT NULL DEFAULT false;
ALTER TABLE agent_settings NO FORCE ROW LEVEL SECURITY;
UPDATE agent_settings SET from_tag = true WHERE changed_by = '' AND rules_page IS NOT NULL;
ALTER TABLE objects NO FORCE ROW LEVEL SECURITY;
UPDATE agent_settings s SET rules_page = NULL, from_tag = false
WHERE s.from_tag AND EXISTS (
    WITH RECURSIVE up AS (
        SELECT id, parent, private FROM objects WHERE tenant = s.tenant AND id = s.rules_page
        UNION ALL SELECT o.id, o.parent, o.private FROM objects o JOIN up ON o.tenant = s.tenant AND o.id = up.parent
    ) SELECT 1 FROM up WHERE private);
ALTER TABLE objects FORCE ROW LEVEL SECURITY;
ALTER TABLE agent_settings FORCE ROW LEVEL SECURITY;

-- The decision of ADR-36 (migrations 030, 035, 037, 038), with the rights a link lends checked on the object asked for.
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
                    -- A link lends the actions of its mode, as far as its maker still may share and still holds the
                    -- action on the object asked for, not only where the link is (T-34, B-6): a deny for the maker
                    -- below the link holds for the link as well.
                    SELECT l.id INTO lid FROM share_links l JOIN share_link_users u ON u.link = l.id
                    WHERE l.object = cur AND u.account = viewer AND l.mode IN ('comment', 'edit') AND act = ANY (preset_actions(l.mode))
                      AND (l.expires IS NULL OR l.expires > now())
                      AND (object_decide(obj, l.created_by, 'share', false, false)).allowed
                      AND (object_decide(obj, l.created_by, act, false, false)).allowed
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
