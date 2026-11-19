-- SPDX-License-Identifier: MIT
-- Roles for persons, groups and the tenant (ADR-29). They replace the one level for all others of ADR-20: an object
-- holds what is granted on it and on everything above it, up to the top or to an object that breaks the inheritance,
-- and of all roles an account holds the highest counts. The owner stays, and private stays private.
--
-- One function, object_role, decides the role of an account on an object; every query that lists, finds or opens
-- objects asks it. Every new table carries its tenant and keeps to it as the tables of 013_tenants.sql do.

-- Groups: local ones, made by administrators with members kept here, and groups taken over from OIDC or the gate, whose
-- members are the accounts whose last sign-in named them (accounts.groups). Groups do not nest.
CREATE TABLE groups (
    tenant      text NOT NULL DEFAULT current_setting('underdoc.tenant', true) REFERENCES tenants (id),
    id          text NOT NULL CHECK (id ~ '^[0-9a-hjkmnp-tv-z]{10}$'),
    name        text NOT NULL CHECK (length(name) BETWEEN 1 AND 200),
    source      text NOT NULL CHECK (source IN ('local', 'synced')),
    description text NOT NULL DEFAULT '' CHECK (length(description) <= 1000),
    created     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant, id),
    UNIQUE (tenant, source, name)
);

CREATE TABLE group_members (
    tenant  text NOT NULL DEFAULT current_setting('underdoc.tenant', true) REFERENCES tenants (id),
    grp     text NOT NULL,
    account text NOT NULL,
    added   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant, grp, account),
    FOREIGN KEY (tenant, grp) REFERENCES groups (tenant, id) ON DELETE CASCADE,
    FOREIGN KEY (tenant, account) REFERENCES accounts (tenant, id) ON DELETE CASCADE
);

CREATE INDEX group_members_by_account ON group_members (tenant, account);

-- A role granted on an object: to a person (an account), a group, or the whole tenant (who is empty). The owner is not
-- a grant but the column objects.owner.
CREATE TABLE grants (
    tenant     text NOT NULL DEFAULT current_setting('underdoc.tenant', true) REFERENCES tenants (id),
    object     text NOT NULL,
    kind       text NOT NULL CHECK (kind IN ('person', 'group', 'tenant')),
    who        text NOT NULL DEFAULT '',
    role       text NOT NULL CHECK (role IN ('read', 'comment', 'edit', 'manage')),
    granted_by text NOT NULL DEFAULT '',
    granted_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant, object, kind, who),
    FOREIGN KEY (tenant, object) REFERENCES objects (tenant, id) ON DELETE CASCADE,
    CHECK ((kind = 'tenant') = (who = ''))
);

CREATE INDEX grants_by_who ON grants (tenant, kind, who);

-- An object that breaks the inheritance takes no roles from above.
ALTER TABLE objects ADD COLUMN inherit boolean NOT NULL DEFAULT true;

-- A new ID in the form of ADR-18, for groups the database makes itself. These IDs are names, not secrets.
CREATE FUNCTION new_id() RETURNS text LANGUAGE sql VOLATILE AS $$
    SELECT string_agg(substr('0123456789abcdefghjkmnpqrstvwxyz', 1 + floor(random() * 32)::integer, 1), '')
    FROM generate_series(1, 10)
$$;

-- The rank of a role: nothing 0, read 1, comment 2, edit 3, manage 4, owner 5.
CREATE FUNCTION role_rank(role text) RETURNS integer LANGUAGE sql IMMUTABLE AS $$
    SELECT CASE role WHEN 'read' THEN 1 WHEN 'comment' THEN 2 WHEN 'edit' THEN 3 WHEN 'manage' THEN 4 WHEN 'owner' THEN 5 ELSE 0 END
$$;

CREATE FUNCTION role_name(rank integer) RETURNS text LANGUAGE sql IMMUTABLE AS $$
    SELECT CASE rank WHEN 1 THEN 'read' WHEN 2 THEN 'comment' WHEN 3 THEN 'edit' WHEN 4 THEN 'manage' WHEN 5 THEN 'owner' ELSE '' END
$$;

-- The groups an account belongs to: local ones by their members, those taken over by the account's last sign-in.
CREATE FUNCTION account_groups(viewer text) RETURNS text[] LANGUAGE sql STABLE AS $$
    SELECT coalesce(array_agg(g.id ORDER BY g.id), '{}') FROM groups g
    WHERE (g.source = 'local' AND EXISTS (SELECT 1 FROM group_members m WHERE m.grp = g.id AND m.account = viewer))
       OR (g.source = 'synced' AND EXISTS (SELECT 1 FROM accounts a WHERE a.id = viewer AND g.name = ANY (a.groups)))
$$;

-- The role of an account on an object, as a rank (ADR-29). Private objects, and everything below them, belong to the owner
-- of the private object alone: anyone else holds nothing there, whatever is granted. Elsewhere the owner holds owner, the
-- owner of something above manages, and each grant on the object and above it counts, up to the top or to the first
-- object that breaks the inheritance.
CREATE FUNCTION object_role(obj text, viewer text) RETURNS integer LANGUAGE plpgsql STABLE AS $$
DECLARE
    cur        text := obj;
    level      record;
    best       integer := 0;
    granted    integer;
    inheriting boolean := true;
    is_self    boolean := true;
    in_private boolean := false;
    known      boolean;
    gs         text[];
BEGIN
    IF viewer IS NULL OR viewer = '' THEN
        RETURN 0;
    END IF;
    SELECT EXISTS (SELECT 1 FROM accounts WHERE id = viewer) INTO known;
    IF NOT known THEN
        RETURN 0;
    END IF;
    gs := account_groups(viewer);
    WHILE cur IS NOT NULL LOOP
        SELECT id, parent, owner, private, inherit INTO level FROM objects WHERE id = cur;
        IF NOT FOUND THEN
            EXIT;
        END IF;
        IF level.private THEN
            IF level.owner <> viewer THEN
                RETURN 0;
            END IF;
            in_private := true;
        END IF;
        IF inheriting THEN
            IF level.owner = viewer THEN
                best := greatest(best, CASE WHEN is_self THEN 5 ELSE 4 END);
            END IF;
            SELECT max(role_rank(g.role)) INTO granted FROM grants g
            WHERE g.object = cur AND (g.kind = 'tenant' OR (g.kind = 'person' AND g.who = viewer) OR (g.kind = 'group' AND g.who = ANY (gs)));
            best := greatest(best, coalesce(granted, 0));
            IF NOT level.inherit THEN
                inheriting := false;
            END IF;
        END IF;
        is_self := false;
        cur := level.parent;
    END LOOP;
    IF in_private THEN
        RETURN 5;
    END IF;
    RETURN best;
END
$$;

-- The role the whole tenant is granted on the object itself, for the level the workspace shows.
CREATE FUNCTION tenant_grant(obj text) RETURNS text LANGUAGE sql STABLE AS $$
    SELECT role FROM grants WHERE object = obj AND kind = 'tenant'
$$;

-- Who may read an object, as reader tokens of the search index: 'all' for the tenant, 'account:<id>', 'group:<id>'.
-- An empty list is read by no one.
CREATE FUNCTION object_readers(obj text) RETURNS text[] LANGUAGE plpgsql STABLE AS $$
DECLARE
    cur        text := obj;
    level      record;
    readers    text[] := '{}';
    inheriting boolean := true;
    owner_of   text := NULL;
BEGIN
    WHILE cur IS NOT NULL LOOP
        SELECT id, parent, owner, private, inherit INTO level FROM objects WHERE id = cur;
        IF NOT FOUND THEN
            EXIT;
        END IF;
        IF level.private THEN
            IF owner_of IS NOT NULL AND owner_of <> level.owner THEN
                RETURN '{}';
            END IF;
            owner_of := level.owner;
        END IF;
        IF inheriting THEN
            readers := readers || ('account:' || level.owner) || ARRAY(
                SELECT CASE g.kind WHEN 'tenant' THEN 'all' ELSE (CASE g.kind WHEN 'person' THEN 'account:' ELSE 'group:' END) || g.who END
                FROM grants g WHERE g.object = cur);
            IF NOT level.inherit THEN
                inheriting := false;
            END IF;
        END IF;
        cur := level.parent;
    END LOOP;
    IF owner_of IS NOT NULL THEN
        RETURN ARRAY['account:' || owner_of];
    END IF;
    RETURN ARRAY(SELECT DISTINCT r FROM unnest(readers) AS r ORDER BY r);
END
$$;

-- The levels of ADR-20 become grants. The level an object held for all others was the first one set on it or above it,
-- else edit. A level at the top becomes a grant to the tenant; below, a level wider than the one above it becomes a grant
-- to the tenant, and a narrower one breaks the inheritance and grants the tenant that level. Private trees get nothing.
-- The rows of all tenants are changed at once, so row-level security steps aside for the owner of the tables until the
-- end of this migration.
ALTER TABLE objects NO FORCE ROW LEVEL SECURITY;
ALTER TABLE file_roots NO FORCE ROW LEVEL SECURITY;
ALTER TABLE accounts NO FORCE ROW LEVEL SECURITY;
-- The index is built anew anyway (the source raises its version), so the queue need not hear of these changes.
ALTER TABLE objects DISABLE TRIGGER objects_to_search;

CREATE TEMPORARY TABLE old_levels ON COMMIT DROP AS
WITH RECURSIVE down AS (
    SELECT tenant, id, access, coalesce(access, 'edit') AS held, NULL::text AS above, private AS hidden
    FROM objects WHERE parent IS NULL
    UNION ALL
    SELECT o.tenant, o.id, o.access, coalesce(o.access, down.held), down.held, down.hidden OR o.private
    FROM objects o JOIN down ON o.tenant = down.tenant AND o.parent = down.id
)
SELECT tenant, id, access, held, above, hidden FROM down;

INSERT INTO grants (tenant, object, kind, who, role)
SELECT tenant, id, 'tenant', '', CASE WHEN above IS NULL THEN held ELSE access END FROM old_levels l
WHERE NOT hidden AND (above IS NULL OR (access IS NOT NULL AND access <> above))
  AND NOT EXISTS (SELECT 1 FROM objects o WHERE o.tenant = l.tenant AND o.id = l.id AND o.kind IN ('line', 'space'));

UPDATE objects o SET inherit = false FROM old_levels l
WHERE o.tenant = l.tenant AND o.id = l.id AND NOT l.hidden AND l.above IS NOT NULL AND l.access IS NOT NULL
  AND role_rank(l.access) < role_rank(l.above);

-- The group of writers of the folder of the tenant (ADR-28) is a group taken over, granted edit there.
INSERT INTO groups (tenant, id, name, source)
SELECT DISTINCT ON (r.tenant) r.tenant, new_id(), r.writers, 'synced'
FROM file_roots r WHERE r.role = 'tenant' AND r.writers <> ''
ON CONFLICT (tenant, source, name) DO NOTHING;

INSERT INTO grants (tenant, object, kind, who, role)
SELECT r.tenant, r.object, 'group', g.id, 'edit' FROM file_roots r JOIN groups g ON g.tenant = r.tenant AND g.source = 'synced' AND g.name = r.writers
WHERE r.role = 'tenant' AND r.writers <> ''
ON CONFLICT DO NOTHING;

-- Every name of a group that an account brought along is a group taken over.
INSERT INTO groups (tenant, id, name, source)
SELECT n.tenant, new_id(), n.name, 'synced'
FROM (SELECT DISTINCT a.tenant, g AS name FROM accounts a, unnest(a.groups) AS g WHERE length(g) BETWEEN 1 AND 200) n
ON CONFLICT (tenant, source, name) DO NOTHING;

ALTER TABLE objects DROP COLUMN access;
ALTER TABLE file_roots DROP COLUMN writers;

ALTER TABLE objects ENABLE TRIGGER objects_to_search;
ALTER TABLE objects FORCE ROW LEVEL SECURITY;
ALTER TABLE file_roots FORCE ROW LEVEL SECURITY;
ALTER TABLE accounts FORCE ROW LEVEL SECURITY;

ALTER TABLE groups ENABLE ROW LEVEL SECURITY;
ALTER TABLE groups FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_only ON groups
    USING (tenant = current_setting('underdoc.tenant', true)) WITH CHECK (tenant = current_setting('underdoc.tenant', true));

ALTER TABLE group_members ENABLE ROW LEVEL SECURITY;
ALTER TABLE group_members FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_only ON group_members
    USING (tenant = current_setting('underdoc.tenant', true)) WITH CHECK (tenant = current_setting('underdoc.tenant', true));

ALTER TABLE grants ENABLE ROW LEVEL SECURITY;
ALTER TABLE grants FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_only ON grants
    USING (tenant = current_setting('underdoc.tenant', true)) WITH CHECK (tenant = current_setting('underdoc.tenant', true));

-- A change of roles, of inheritance or of the owner changes who may read the object and everything below it.
CREATE OR REPLACE FUNCTION search_queue_object() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        PERFORM search_enqueue(OLD.tenant, ARRAY[OLD.id]);
        RETURN OLD;
    END IF;
    IF TG_OP = 'UPDATE' AND (NEW.title, NEW.parent, NEW.private, NEW.owner, NEW.inherit) IS NOT DISTINCT FROM
            (OLD.title, OLD.parent, OLD.private, OLD.owner, OLD.inherit) THEN
        PERFORM search_enqueue(NEW.tenant, ARRAY[NEW.id]);
        RETURN NEW;
    END IF;
    PERFORM search_enqueue(NEW.tenant, ARRAY(
        WITH RECURSIVE down AS (
            SELECT NEW.id AS id
            UNION ALL SELECT o.id FROM objects o JOIN down ON o.tenant = NEW.tenant AND o.parent = down.id
        )
        SELECT id FROM down));
    RETURN NEW;
END
$$;

CREATE FUNCTION search_queue_grant() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    of_tenant text := CASE WHEN TG_OP = 'DELETE' THEN OLD.tenant ELSE NEW.tenant END;
    obj       text := CASE WHEN TG_OP = 'DELETE' THEN OLD.object ELSE NEW.object END;
BEGIN
    PERFORM search_enqueue(of_tenant, ARRAY(
        WITH RECURSIVE down AS (
            SELECT obj AS id
            UNION ALL SELECT o.id FROM objects o JOIN down ON o.tenant = of_tenant AND o.parent = down.id
        )
        SELECT id FROM down));
    RETURN NULL;
END
$$;

CREATE TRIGGER grants_to_search AFTER INSERT OR UPDATE OR DELETE ON grants
    FOR EACH ROW EXECUTE FUNCTION search_queue_grant();
