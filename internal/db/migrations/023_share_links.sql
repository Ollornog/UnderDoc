-- SPDX-License-Identifier: MIT
-- Share links (ADR-29): a link shares one object and what is below it through a token in its address; only the SHA-256
-- of the token is kept. Read is for anyone with the link and shows a page frozen; upload only is for a folder of the
-- files; comment and edit are for accounts of the tenant, who hold that role on the object once they opened the link
-- while signed in, as long as the link lives.

CREATE TABLE share_links (
    tenant     text NOT NULL DEFAULT current_setting('underdoc.tenant', true) REFERENCES tenants (id),
    id         text NOT NULL CHECK (id ~ '^[0-9a-hjkmnp-tv-z]{10}$'),
    object     text NOT NULL,
    token      bytea NOT NULL CHECK (length(token) = 32),
    mode       text NOT NULL CHECK (mode IN ('read', 'comment', 'edit', 'upload')),
    -- An Argon2id hash, empty for a link without a password.
    password   text NOT NULL DEFAULT '',
    expires    timestamptz,
    created_by text NOT NULL,
    created    timestamptz NOT NULL DEFAULT now(),
    -- A page as it was rendered when the link was made or last renewed, with the embedded objects released then.
    frozen     text,
    frozen_at  timestamptz,
    released   text[] NOT NULL DEFAULT '{}',
    PRIMARY KEY (tenant, id),
    UNIQUE (tenant, token),
    FOREIGN KEY (tenant, object) REFERENCES objects (tenant, id) ON DELETE CASCADE,
    FOREIGN KEY (tenant, created_by) REFERENCES accounts (tenant, id)
);

CREATE INDEX share_links_by_object ON share_links (tenant, object);

-- The accounts that opened a link to comment or edit while signed in.
CREATE TABLE share_link_users (
    tenant  text NOT NULL DEFAULT current_setting('underdoc.tenant', true) REFERENCES tenants (id),
    link    text NOT NULL,
    account text NOT NULL,
    since   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant, link, account),
    FOREIGN KEY (tenant, link) REFERENCES share_links (tenant, id) ON DELETE CASCADE,
    FOREIGN KEY (tenant, account) REFERENCES accounts (tenant, id) ON DELETE CASCADE
);

-- A browser that gave the password of a link: the SHA-256 of a random value in its cookie, for a day.
CREATE TABLE share_link_passes (
    tenant  text NOT NULL DEFAULT current_setting('underdoc.tenant', true) REFERENCES tenants (id),
    pass    bytea NOT NULL CHECK (length(pass) = 32),
    link    text NOT NULL,
    expires timestamptz NOT NULL,
    PRIMARY KEY (tenant, pass),
    FOREIGN KEY (tenant, link) REFERENCES share_links (tenant, id) ON DELETE CASCADE
);

ALTER TABLE share_links ENABLE ROW LEVEL SECURITY;
ALTER TABLE share_links FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_only ON share_links
    USING (tenant = current_setting('underdoc.tenant', true)) WITH CHECK (tenant = current_setting('underdoc.tenant', true));

ALTER TABLE share_link_users ENABLE ROW LEVEL SECURITY;
ALTER TABLE share_link_users FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_only ON share_link_users
    USING (tenant = current_setting('underdoc.tenant', true)) WITH CHECK (tenant = current_setting('underdoc.tenant', true));

ALTER TABLE share_link_passes ENABLE ROW LEVEL SECURITY;
ALTER TABLE share_link_passes FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_only ON share_link_passes
    USING (tenant = current_setting('underdoc.tenant', true)) WITH CHECK (tenant = current_setting('underdoc.tenant', true));

-- The role of an account now also counts the links to comment or edit it opened, on the object of the link and below it,
-- while the link has not expired and its maker still manages its object (counted without links, so a link never lends
-- a role on the strength of another link). Private objects never have links.
CREATE FUNCTION object_role(obj text, viewer text, with_links boolean) RETURNS integer LANGUAGE plpgsql STABLE AS $$
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
            IF with_links THEN
                SELECT max(role_rank(l.mode)) INTO granted FROM share_links l JOIN share_link_users u ON u.link = l.id
                WHERE l.object = cur AND u.account = viewer AND l.mode IN ('comment', 'edit') AND (l.expires IS NULL OR l.expires > now())
                  AND object_role(l.object, l.created_by, false) >= 4;
                best := greatest(best, coalesce(granted, 0));
            END IF;
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

CREATE OR REPLACE FUNCTION object_role(obj text, viewer text) RETURNS integer LANGUAGE sql STABLE AS $$
    SELECT object_role(obj, viewer, true)
$$;
