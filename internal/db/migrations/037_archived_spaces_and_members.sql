-- SPDX-License-Identifier: MIT
-- Archived spaces and the preset Members (T-34, ADR-36).
--
-- * A space, a collection at the top of the documents or of Files, can be archived: it stays with everything inside
--   it, without a limit, hidden from the tree and the search unless asked for, and read only. Archived, every action
--   but read and manage is refused for everyone, the owner, administrators and agents included: no comment, create,
--   write, delete or share. The one check decides it, so every door holds it: the web, the API, MCP, Samba, share
--   links and upload links (a link lends nothing while its maker may not share). Manage stays, so who may read the
--   archive can still be set; the owner and administrators archive, restore and transfer it.
-- * The preset Members: every action but delete. Whoever holds it manages members and rights, but deletes nothing and
--   archives nothing (only the owner and administrators archive).
-- * A link to comment or edit lends only the actions its maker still holds: whoever manages without delete cannot pass
--   delete on through a link.

ALTER TABLE objects ADD COLUMN archived timestamptz;
ALTER TABLE objects ADD COLUMN archived_by text NOT NULL DEFAULT '';
-- Only a space is archived: a collection at the top that is not private. Moving it elsewhere is refused by the check
-- before; this keeps it true whatever a change does.
ALTER TABLE objects ADD CONSTRAINT objects_archived_space
    CHECK (archived IS NULL OR (parent IS NULL AND kind = 'collection' AND NOT private));

-- The space an object lies in, when it is archived; NULL otherwise.
CREATE FUNCTION object_archive(obj text) RETURNS text LANGUAGE sql STABLE AS $$
    WITH RECURSIVE up AS (
        SELECT id, parent, archived FROM objects WHERE id = obj
        UNION ALL SELECT p.id, p.parent, p.archived FROM objects p JOIN up ON p.id = up.parent
    )
    SELECT id FROM up WHERE parent IS NULL AND archived IS NOT NULL
$$;

-- Whether an object lies in an archived space.
CREATE FUNCTION object_archived(obj text) RETURNS boolean LANGUAGE sql STABLE AS $$
    SELECT object_archive(obj) IS NOT NULL
$$;

CREATE OR REPLACE FUNCTION preset_actions(role text) RETURNS text[] LANGUAGE sql IMMUTABLE AS $$
    SELECT CASE role
        WHEN 'read' THEN ARRAY['read']
        WHEN 'comment' THEN ARRAY['read', 'comment']
        WHEN 'edit' THEN ARRAY['read', 'comment', 'create', 'write', 'delete']
        WHEN 'members' THEN ARRAY['read', 'comment', 'create', 'write', 'share', 'manage']
        WHEN 'manage' THEN ARRAY['read', 'comment', 'create', 'write', 'delete', 'share', 'manage']
        WHEN 'owner' THEN ARRAY['read', 'comment', 'create', 'write', 'delete', 'share', 'manage']
        ELSE '{}'::text[] END
$$;

CREATE OR REPLACE FUNCTION entries_preset(obj text, k text, w text) RETURNS text LANGUAGE sql STABLE AS $$
    WITH e AS (SELECT action, value FROM grants WHERE object = obj AND kind = k AND who = w)
    SELECT CASE
        WHEN NOT EXISTS (SELECT 1 FROM e) THEN ''
        WHEN EXISTS (SELECT 1 FROM e WHERE value = 'deny') THEN 'custom'
        ELSE coalesce((SELECT p FROM unnest(ARRAY['read', 'comment', 'edit', 'members', 'manage']) AS p
            WHERE (SELECT array_agg(action ORDER BY action) FROM e) = (SELECT array_agg(x ORDER BY x) FROM unnest(preset_actions(p)) AS x)), 'custom')
        END
$$;

-- The role the interface shows: whoever may change the content (Members, or edit without delete) works as an editor
-- does; the server still refuses what is not held.
CREATE OR REPLACE FUNCTION object_role(obj text, viewer text, with_links boolean) RETURNS integer LANGUAGE plpgsql STABLE AS $$
DECLARE
    first right_decision;
    held  text[];
BEGIN
    first := object_decide(obj, viewer, 'read', request_agent(), with_links);
    IF first.allowed AND first.via IN ('owner', 'private') AND NOT request_agent() AND NOT object_archived(obj) THEN
        RETURN 5;
    END IF;
    held := object_actions(obj, viewer, request_agent(), with_links);
    RETURN CASE
        WHEN held @> preset_actions('manage') THEN 4
        WHEN held @> ARRAY['read', 'comment', 'create', 'write'] THEN 3
        WHEN held @> preset_actions('comment') THEN 2
        WHEN held @> preset_actions('read') THEN 1
        ELSE 0 END;
END
$$;

-- The decision of ADR-36 (migrations 030, 035), with archived spaces and links that lend only what their maker holds.
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
