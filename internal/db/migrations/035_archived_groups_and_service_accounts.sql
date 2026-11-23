-- SPDX-License-Identifier: MIT
-- Archived groups and service accounts (B-3, ADR-35, ADR-36).
--
-- * A local group is archived instead of deleted: it keeps its members, its groups and every entry granted to it, so
--   restoring it brings everything back. While it is archived, its allow entries hold for nobody, and nothing passes
--   up through it. Its deny entries keep holding for whoever was in it: archiving never gives anyone more.
-- * An account can be a service account: the gate says so and an administrator has allowed its lasting ID. A service
--   account is always an agent, and it never belongs to the audience everyone signed in (the tenant), nor to a role of
--   the gate (a group taken over), which may name everyone too. It gets what is granted to it, to it alone or to a local
--   group it is put in. A deny for the tenant or a group taken over still holds for it.

ALTER TABLE groups ADD COLUMN archived timestamptz;
ALTER TABLE groups ADD COLUMN archived_by text NOT NULL DEFAULT '';

ALTER TABLE accounts ADD COLUMN service boolean NOT NULL DEFAULT false;

-- The share decides groups by the entries; an archived group changes them.
CREATE TRIGGER share_notify AFTER UPDATE OF archived ON groups FOR EACH ROW EXECUTE FUNCTION share_notify();

-- The check of groups inside groups (migration 030), which also keeps archived groups out: nothing is put inside an
-- archived group, and no archived group is put inside another. It runs under the lock of the tenant that archiving takes
-- too, so a group cannot get a group inside while it is being archived.
CREATE OR REPLACE FUNCTION group_groups_check() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    PERFORM pg_advisory_xact_lock(hashtext('underdoc groups ' || NEW.tenant));
    IF NOT EXISTS (SELECT 1 FROM groups WHERE tenant = NEW.tenant AND id = NEW.grp AND source = 'local') THEN
        RAISE EXCEPTION 'only a local group contains groups' USING ERRCODE = 'check_violation', CONSTRAINT = 'group_groups_local';
    END IF;
    IF EXISTS (SELECT 1 FROM groups WHERE tenant = NEW.tenant AND id IN (NEW.grp, NEW.member) AND archived IS NOT NULL) THEN
        RAISE EXCEPTION 'an archived group neither contains groups nor goes inside one' USING ERRCODE = 'check_violation',
            CONSTRAINT = 'group_groups_archived';
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

-- Whether an account is a service account. An unknown account is none.
CREATE FUNCTION account_service(viewer text) RETURNS boolean LANGUAGE sql STABLE AS $$
    SELECT coalesce((SELECT service FROM accounts WHERE id = viewer), false)
$$;

-- The groups whose allow entries hold for an account, directly: those of account_direct_groups that are not archived,
-- and for a service account only the local ones.
CREATE FUNCTION account_allow_direct_groups(viewer text) RETURNS text[] LANGUAGE sql STABLE AS $$
    SELECT coalesce(array_agg(g.id ORDER BY g.id), '{}') FROM groups g
    WHERE g.archived IS NULL AND (
           (g.source = 'local' AND EXISTS (SELECT 1 FROM group_members m WHERE m.grp = g.id AND m.account = viewer))
        OR (g.source = 'synced' AND NOT account_service(viewer)
            AND EXISTS (SELECT 1 FROM accounts a WHERE a.id = viewer AND g.name = ANY (a.groups))))
$$;

-- All of them: the direct ones, and every local group that contains one of those, at any depth, never through an
-- archived group.
CREATE FUNCTION account_allow_groups(viewer text) RETURNS text[] LANGUAGE sql STABLE AS $$
    WITH RECURSIVE up(id) AS (
        SELECT unnest(account_allow_direct_groups(viewer))
        UNION SELECT gg.grp FROM group_groups gg JOIN up ON gg.member = up.id
            JOIN groups g ON g.id = gg.grp WHERE g.archived IS NULL
    )
    SELECT coalesce(array_agg(id ORDER BY id), '{}') FROM up
$$;

-- The decision of ADR-36 (migration 030), with archived groups and service accounts. A deny is matched against every
-- group of the account (account_groups), archived ones included; an allow only against the groups whose allow holds
-- (account_allow_groups). An allow for the tenant never holds for a service account, its deny does. A service account is
-- an agent whatever the caller says, so its ceilings hold through every door.
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

-- The secrets of a page (ADR-7, migration 030): a restricted secret is read by the groups named on it only while they
-- are not archived, and never by a service account, which is an agent.
CREATE OR REPLACE FUNCTION secret_may(sec text, viewer text) RETURNS boolean LANGUAGE sql STABLE AS $$
    SELECT NOT request_agent() AND NOT account_service(viewer) AND EXISTS (
        SELECT 1 FROM page_secrets s JOIN objects o ON o.id = s.page
        WHERE s.id = sec AND o.deleted_at IS NULL AND (object_decide(s.page, viewer, 'read', request_agent(), false)).allowed
          AND (NOT s.restricted OR EXISTS (
              SELECT 1 FROM page_secret_readers r WHERE r.secret = s.id
                AND ((r.kind = 'person' AND r.who = viewer) OR (r.kind = 'group' AND r.who = ANY (account_allow_groups(viewer))))))
    )
$$;
