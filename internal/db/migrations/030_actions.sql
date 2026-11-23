-- SPDX-License-Identifier: MIT
-- Rights per action with allow and deny, nested groups and the built-in group Agents (ADR-36). It replaces the parts of
-- ADR-29 that kept groups flat and knew only allowing:
--
-- * A grant is no longer one role but one row per action (read, comment, create, write, delete, share, manage), each
--   allow or deny. No row is inherit. The roles of ADR-29 stay as presets of actions.
-- * Local groups contain persons, other local groups and groups taken over from the sign-in. Membership is passed down;
--   a grant to a group holds for its subgroups unless the grant says otherwise (subgroups).
-- * For each action the nearest object with an entry for the account or any of its groups decides, and on that object
--   deny beats allow. Without an entry the action is denied. The owner keeps every action on their own object, and
--   private stays private.
-- * The group Agents holds every request that did not sign in as an interactive session. A deny on it is a ceiling
--   after the evaluation, also for the owner.
--
-- Existing grants become allow entries of their preset, and an object that broke the inheritance still breaks it.

-- --- Groups inside groups, and the admins of a group.

CREATE TABLE group_groups (
    tenant text NOT NULL DEFAULT current_setting('underdoc.tenant', true) REFERENCES tenants (id),
    grp    text NOT NULL,
    member text NOT NULL,
    added  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant, grp, member),
    FOREIGN KEY (tenant, grp) REFERENCES groups (tenant, id) ON DELETE CASCADE,
    FOREIGN KEY (tenant, member) REFERENCES groups (tenant, id) ON DELETE CASCADE,
    CHECK (grp <> member)
);

CREATE INDEX group_groups_by_member ON group_groups (tenant, member);

CREATE TABLE group_admins (
    tenant  text NOT NULL DEFAULT current_setting('underdoc.tenant', true) REFERENCES tenants (id),
    grp     text NOT NULL,
    account text NOT NULL,
    added   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant, grp, account),
    FOREIGN KEY (tenant, grp) REFERENCES groups (tenant, id) ON DELETE CASCADE,
    FOREIGN KEY (tenant, account) REFERENCES accounts (tenant, id) ON DELETE CASCADE
);

CREATE INDEX group_admins_by_account ON group_admins (tenant, account);

-- Only a local group contains groups (roles of the gate do not nest, ADR-35), and no group contains itself through
-- others. The check runs under a lock of the tenant, so two changes at once cannot close a cycle between them.
CREATE FUNCTION group_groups_check() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    PERFORM pg_advisory_xact_lock(hashtext('underdoc groups ' || NEW.tenant));
    IF NOT EXISTS (SELECT 1 FROM groups WHERE tenant = NEW.tenant AND id = NEW.grp AND source = 'local') THEN
        RAISE EXCEPTION 'only a local group contains groups' USING ERRCODE = 'check_violation', CONSTRAINT = 'group_groups_local';
    END IF;
    IF EXISTS (
        WITH RECURSIVE down(id) AS (
            SELECT NEW.member
            UNION SELECT gg.member FROM group_groups gg JOIN down ON gg.tenant = NEW.tenant AND gg.grp = down.id
        ) SELECT 1 FROM down WHERE id = NEW.grp
    ) THEN
        RAISE EXCEPTION 'a group cannot contain itself' USING ERRCODE = 'check_violation', CONSTRAINT = 'group_groups_cycle';
    END IF;
    RETURN NEW;
END
$$;

CREATE TRIGGER group_groups_check BEFORE INSERT OR UPDATE ON group_groups FOR EACH ROW EXECUTE FUNCTION group_groups_check();

-- --- What each person lets their own agents do. No row: everything undoable (read, comment, create, write, delete).

CREATE TABLE agent_limits (
    tenant  text NOT NULL DEFAULT current_setting('underdoc.tenant', true) REFERENCES tenants (id),
    account text NOT NULL,
    actions text[] NOT NULL CHECK (actions <@ ARRAY['read', 'comment', 'create', 'write', 'delete']),
    changed timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant, account),
    FOREIGN KEY (tenant, account) REFERENCES accounts (tenant, id) ON DELETE CASCADE
);

ALTER TABLE group_groups ENABLE ROW LEVEL SECURITY;
ALTER TABLE group_groups FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_only ON group_groups
    USING (tenant = current_setting('underdoc.tenant', true)) WITH CHECK (tenant = current_setting('underdoc.tenant', true));

ALTER TABLE group_admins ENABLE ROW LEVEL SECURITY;
ALTER TABLE group_admins FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_only ON group_admins
    USING (tenant = current_setting('underdoc.tenant', true)) WITH CHECK (tenant = current_setting('underdoc.tenant', true));

ALTER TABLE agent_limits ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_limits FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_only ON agent_limits
    USING (tenant = current_setting('underdoc.tenant', true)) WITH CHECK (tenant = current_setting('underdoc.tenant', true));

-- --- The groups of an account.

-- Directly: the local groups that list it, and the groups taken over that its last sign-in named.
CREATE FUNCTION account_direct_groups(viewer text) RETURNS text[] LANGUAGE sql STABLE AS $$
    SELECT coalesce(array_agg(g.id ORDER BY g.id), '{}') FROM groups g
    WHERE (g.source = 'local' AND EXISTS (SELECT 1 FROM group_members m WHERE m.grp = g.id AND m.account = viewer))
       OR (g.source = 'synced' AND EXISTS (SELECT 1 FROM accounts a WHERE a.id = viewer AND g.name = ANY (a.groups)))
$$;

-- All of them: the direct ones, and every local group that contains one of those, at any depth. UNION stops at a group
-- seen before, so even a cycle that slipped in ends.
CREATE OR REPLACE FUNCTION account_groups(viewer text) RETURNS text[] LANGUAGE sql STABLE AS $$
    WITH RECURSIVE up(id) AS (
        SELECT unnest(account_direct_groups(viewer))
        UNION SELECT gg.grp FROM group_groups gg JOIN up ON gg.member = up.id
    )
    SELECT coalesce(array_agg(id ORDER BY id), '{}') FROM up
$$;

-- --- Grants become entries per action.

-- The presets: the roles of ADR-29 as sets of actions.
CREATE FUNCTION preset_actions(role text) RETURNS text[] LANGUAGE sql IMMUTABLE AS $$
    SELECT CASE role
        WHEN 'read' THEN ARRAY['read']
        WHEN 'comment' THEN ARRAY['read', 'comment']
        WHEN 'edit' THEN ARRAY['read', 'comment', 'create', 'write', 'delete']
        WHEN 'manage' THEN ARRAY['read', 'comment', 'create', 'write', 'delete', 'share', 'manage']
        WHEN 'owner' THEN ARRAY['read', 'comment', 'create', 'write', 'delete', 'share', 'manage']
        ELSE '{}'::text[] END
$$;

ALTER TABLE grants NO FORCE ROW LEVEL SECURITY;
-- The index is built anew (the source raises its version), and the share hears of it once at the next change.
ALTER TABLE grants DISABLE TRIGGER grants_to_search;
ALTER TABLE grants DISABLE TRIGGER share_notify;

ALTER TABLE grants DROP CONSTRAINT grants_pkey;
ALTER TABLE grants DROP CONSTRAINT grants_kind_check;
ALTER TABLE grants DROP CONSTRAINT grants_check;
ALTER TABLE grants ADD COLUMN action text;
ALTER TABLE grants ADD COLUMN value text NOT NULL DEFAULT 'allow';
ALTER TABLE grants ADD COLUMN subgroups boolean NOT NULL DEFAULT true;

INSERT INTO grants (tenant, object, kind, who, role, granted_by, granted_at, action, value, subgroups)
SELECT g.tenant, g.object, g.kind, g.who, g.role, g.granted_by, g.granted_at, a, 'allow', true
FROM grants g CROSS JOIN LATERAL unnest(preset_actions(g.role)) AS a
WHERE g.action IS NULL;
DELETE FROM grants WHERE action IS NULL;

ALTER TABLE grants DROP COLUMN role;
ALTER TABLE grants ALTER COLUMN action SET NOT NULL;
ALTER TABLE grants ALTER COLUMN value DROP DEFAULT;
ALTER TABLE grants ADD PRIMARY KEY (tenant, object, kind, who, action);
ALTER TABLE grants ADD CONSTRAINT grants_kind_check CHECK (kind IN ('person', 'group', 'tenant', 'agents'));
ALTER TABLE grants ADD CONSTRAINT grants_who_check CHECK ((kind IN ('tenant', 'agents')) = (who = ''));
ALTER TABLE grants ADD CONSTRAINT grants_action_check
    CHECK (action IN ('read', 'comment', 'create', 'write', 'delete', 'share', 'manage'));
ALTER TABLE grants ADD CONSTRAINT grants_value_check CHECK (value IN ('allow', 'deny'));
-- Agents only ever narrow: an allow for every agent of every person would give agents what their persons do not hold.
ALTER TABLE grants ADD CONSTRAINT grants_agents_deny CHECK (kind <> 'agents' OR value = 'deny');
-- Only a grant to a group knows subgroups.
ALTER TABLE grants ADD CONSTRAINT grants_subgroups_check CHECK (kind = 'group' OR subgroups);

ALTER TABLE grants ENABLE TRIGGER grants_to_search;
ALTER TABLE grants ENABLE TRIGGER share_notify;
ALTER TABLE grants FORCE ROW LEVEL SECURITY;

-- --- The one decision.

-- What decided an action of an account on an object, and whether it is allowed:
--   via     owner (of the object itself), owner above, entry, link, private (the owner of something private, or
--           someone else in it), none (no entry up to the top), agents (a deny on Agents), agents default (what agents
--           never do), agents own (what the person does not let their agents do), unknown (no such account or action);
--   object  where it was found, kind and who the entry (person, group, tenant, agents, link), value allow or deny.
CREATE TYPE right_decision AS (allowed boolean, via text, object text, kind text, who text, value text);

-- The decision of ADR-36. For the action, walk up from the object while it inherits; the first object with an entry
-- for the account, a group of it (with its subgroups unless the entry says otherwise) or the tenant decides, deny before
-- allow. The owner of an object holds every action on it; the owner of something above holds every action there, as an
-- allow on that level, which a deny on the same level beats. A share link to comment or edit that the account opened
-- counts as an allow of its preset on the link's object while its maker may still share. Private objects belong to the
-- owner of the private object alone. For an agent, the ceilings come last: never manage or share, never what a deny on
-- Agents on the object or anywhere above forbids (whatever breaks the inheritance), never what the person does not let
-- their agents do.
CREATE FUNCTION object_decide(obj text, viewer text, act text, agent boolean, with_links boolean) RETURNS right_decision
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
    lid        text;
    limited    text[];
BEGIN
    IF viewer IS NULL OR viewer = '' OR act IS NULL
            OR act NOT IN ('read', 'comment', 'create', 'write', 'delete', 'share', 'manage')
            OR NOT EXISTS (SELECT 1 FROM accounts WHERE id = viewer) THEN
        RETURN ROW(false, 'unknown', '', '', '', '')::right_decision;
    END IF;
    gs := account_groups(viewer);
    gd := account_direct_groups(viewer);
    WHILE cur IS NOT NULL LOOP
        SELECT id, parent, owner, private, inherit INTO level FROM objects WHERE id = cur;
        IF NOT FOUND THEN
            EXIT;
        END IF;
        IF level.private THEN
            IF level.owner <> viewer THEN
                RETURN ROW(false, 'private', level.id, 'person', level.owner, '')::right_decision;
            END IF;
            in_private := true;
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
                  AND (g.kind = 'tenant' OR (g.kind = 'person' AND g.who = viewer)
                       OR (g.kind = 'group' AND g.who = ANY (CASE WHEN g.subgroups THEN gs ELSE gd END)))
                ORDER BY g.value = 'deny' DESC, CASE g.kind WHEN 'person' THEN 0 WHEN 'group' THEN 1 ELSE 2 END, g.who
                LIMIT 1;
                IF FOUND THEN
                    d := ROW(e.value = 'allow', 'entry', cur, e.kind, e.who, e.value)::right_decision;
                    decided := true;
                ELSIF level.owner = viewer THEN
                    d := ROW(true, 'owner above', cur, 'person', viewer, 'allow')::right_decision;
                    decided := true;
                ELSIF with_links THEN
                    SELECT l.id INTO lid FROM share_links l JOIN share_link_users u ON u.link = l.id
                    WHERE l.object = cur AND u.account = viewer AND l.mode IN ('comment', 'edit') AND act = ANY (preset_actions(l.mode))
                      AND (l.expires IS NULL OR l.expires > now())
                      AND (object_decide(l.object, l.created_by, 'share', false, false)).allowed
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

-- Whether the request is one of an agent: the application sets it on every connection it takes for a request
-- (package db, PrepareConn), from the sign-in of the request. Without a request it is off.
CREATE FUNCTION request_agent() RETURNS boolean LANGUAGE sql STABLE AS $$
    SELECT coalesce(current_setting('underdoc.agent', true), '') = 'on'
$$;

-- Whether the account of the request may do the action: the check every query that lists, finds or opens objects asks.
CREATE FUNCTION object_may(obj text, viewer text, act text) RETURNS boolean LANGUAGE sql STABLE AS $$
    SELECT (object_decide(obj, viewer, act, request_agent(), true)).allowed
$$;

-- The actions an account may do on an object, in the order of the table of ADR-36.
CREATE FUNCTION object_actions(obj text, viewer text, agent boolean, with_links boolean) RETURNS text[] LANGUAGE sql STABLE AS $$
    SELECT coalesce(array_agg(a ORDER BY n), '{}') FROM unnest(ARRAY['read', 'comment', 'create', 'write', 'delete', 'share', 'manage'])
        WITH ORDINALITY AS x(a, n)
    WHERE (object_decide(obj, viewer, a, agent, with_links)).allowed
$$;

-- The role of ADR-29 an account holds, for what still speaks of roles: owner where it owns the object or the private
-- tree it lies in, else the widest preset all of whose actions it may do, else nothing. It decides nothing on its own.
CREATE OR REPLACE FUNCTION object_role(obj text, viewer text, with_links boolean) RETURNS integer LANGUAGE plpgsql STABLE AS $$
DECLARE
    first right_decision;
    held  text[];
BEGIN
    first := object_decide(obj, viewer, 'read', request_agent(), with_links);
    IF first.allowed AND first.via IN ('owner', 'private') AND NOT request_agent() THEN
        RETURN 5;
    END IF;
    held := object_actions(obj, viewer, request_agent(), with_links);
    RETURN CASE
        WHEN held @> preset_actions('manage') THEN 4
        WHEN held @> preset_actions('edit') THEN 3
        WHEN held @> preset_actions('comment') THEN 2
        WHEN held @> preset_actions('read') THEN 1
        ELSE 0 END;
END
$$;

CREATE OR REPLACE FUNCTION object_role(obj text, viewer text) RETURNS integer LANGUAGE sql STABLE AS $$
    SELECT object_role(obj, viewer, true)
$$;

-- The preset the entries of one holder on an object make: a role of ADR-29 where they are exactly its actions, all
-- allowed; 'custom' where they are anything else; '' where there are none.
CREATE FUNCTION entries_preset(obj text, k text, w text) RETURNS text LANGUAGE sql STABLE AS $$
    WITH e AS (SELECT action, value FROM grants WHERE object = obj AND kind = k AND who = w)
    SELECT CASE
        WHEN NOT EXISTS (SELECT 1 FROM e) THEN ''
        WHEN EXISTS (SELECT 1 FROM e WHERE value = 'deny') THEN 'custom'
        ELSE coalesce((SELECT p FROM unnest(ARRAY['read', 'comment', 'edit', 'manage']) AS p
            WHERE (SELECT array_agg(action ORDER BY action) FROM e) = (SELECT array_agg(x ORDER BY x) FROM unnest(preset_actions(p)) AS x)), 'custom')
        END
$$;

-- The role the whole tenant is granted on the object itself, for the level the workspace shows.
CREATE OR REPLACE FUNCTION tenant_grant(obj text) RETURNS text LANGUAGE sql STABLE AS $$
    SELECT nullif(entries_preset(obj, 'tenant', ''), '')
$$;

-- The secrets of a page (ADR-7) are revealed through the account's own right to read the page, never through a link,
-- and never by an agent (ADR-36).
CREATE OR REPLACE FUNCTION secret_may(sec text, viewer text) RETURNS boolean LANGUAGE sql STABLE AS $$
    SELECT NOT request_agent() AND EXISTS (
        SELECT 1 FROM page_secrets s JOIN objects o ON o.id = s.page
        WHERE s.id = sec AND o.deleted_at IS NULL AND (object_decide(s.page, viewer, 'read', request_agent(), false)).allowed
          AND (NOT s.restricted OR EXISTS (
              SELECT 1 FROM page_secret_readers r WHERE r.secret = s.id
                AND ((r.kind = 'person' AND r.who = viewer) OR (r.kind = 'group' AND r.who = ANY (account_groups(viewer))))))
    )
$$;

-- Who may read an object, as the search index keeps it (ADR-24, ADR-36): its owner, who always does; and, from the object
-- up while it inherits, each object with entries on read, with the tokens allowed and denied there ('all' for the
-- tenant, 'account:<id>', 'group:<id>' for a group with its subgroups, 'direct:<id>' for its direct members), the
-- owner of each object above counting as allowed on its level; and whether a deny on read for Agents holds on the
-- object or anywhere above. A private object is read by the owner of the private tree alone. The application prunes
-- what can never decide and keeps at most a few levels (search.MaxLevels).
CREATE FUNCTION object_access(obj text) RETURNS jsonb LANGUAGE plpgsql STABLE AS $$
DECLARE
    cur        text := obj;
    level      record;
    levels     jsonb := '[]';
    inheriting boolean := true;
    is_self    boolean := true;
    owner_of   text := NULL;
    self_owner text := '';
    no_agents  boolean := false;
BEGIN
    WHILE cur IS NOT NULL LOOP
        SELECT id, parent, owner, private, inherit INTO level FROM objects WHERE id = cur;
        IF NOT FOUND THEN
            EXIT;
        END IF;
        IF is_self THEN
            self_owner := level.owner;
        END IF;
        IF level.private THEN
            IF owner_of IS NOT NULL AND owner_of <> level.owner THEN
                RETURN jsonb_build_object('owner', '', 'levels', '[]'::jsonb, 'no_agents', false);
            END IF;
            owner_of := level.owner;
        END IF;
        IF EXISTS (SELECT 1 FROM grants g WHERE g.object = cur AND g.kind = 'agents' AND g.action = 'read') THEN
            no_agents := true;
        END IF;
        IF inheriting THEN
            levels := levels || jsonb_build_array(jsonb_build_object(
                'a', to_jsonb(ARRAY(
                    SELECT t FROM (
                        SELECT CASE g.kind WHEN 'tenant' THEN 'all' WHEN 'person' THEN 'account:' || g.who
                            ELSE (CASE WHEN g.subgroups THEN 'group:' ELSE 'direct:' END) || g.who END AS t
                        FROM grants g WHERE g.object = cur AND g.action = 'read' AND g.value = 'allow' AND g.kind <> 'agents'
                        UNION SELECT 'account:' || level.owner WHERE NOT is_self
                    ) x ORDER BY t)),
                'd', to_jsonb(ARRAY(
                    SELECT CASE g.kind WHEN 'tenant' THEN 'all' WHEN 'person' THEN 'account:' || g.who
                        ELSE (CASE WHEN g.subgroups THEN 'group:' ELSE 'direct:' END) || g.who END
                    FROM grants g WHERE g.object = cur AND g.action = 'read' AND g.value = 'deny' AND g.kind <> 'agents' ORDER BY 1))));
            IF NOT level.inherit THEN
                inheriting := false;
            END IF;
        END IF;
        is_self := false;
        cur := level.parent;
    END LOOP;
    IF owner_of IS NOT NULL THEN
        RETURN jsonb_build_object('owner', owner_of, 'levels', '[]'::jsonb, 'no_agents', false);
    END IF;
    RETURN jsonb_build_object('owner', self_owner, 'levels', levels, 'no_agents', no_agents);
END
$$;

DROP FUNCTION object_readers(text);

-- A change of a group's groups or of a person's limits changes what is allowed; the share hears of it as of members.
CREATE TRIGGER share_notify AFTER INSERT OR UPDATE OR DELETE ON group_groups FOR EACH ROW EXECUTE FUNCTION share_notify();
