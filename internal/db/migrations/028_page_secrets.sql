-- SPDX-License-Identifier: MIT
-- Passwords in the text (ADR-7): the text of a page holds only ::secret{id="…"}; the value lives here, sealed by the vault
-- (AES-256-GCM, UNDERDOC_SECRET_KEY), each version on its own and bound to its tenant, secret and number. A secret belongs
-- to exactly one page and goes with it when the page is purged.

CREATE TABLE page_secrets (
    tenant     text NOT NULL DEFAULT current_setting('underdoc.tenant', true) REFERENCES tenants (id),
    id         text NOT NULL CHECK (id ~ '^[0-9a-hjkmnp-tv-z]{10}$'),
    page       text NOT NULL,
    -- restricted: only the persons and groups in page_secret_readers reveal it, and only while they read the page.
    restricted boolean NOT NULL DEFAULT false,
    created_by text NOT NULL,
    created    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant, id),
    FOREIGN KEY (tenant, page) REFERENCES objects (tenant, id) ON DELETE CASCADE,
    FOREIGN KEY (tenant, created_by) REFERENCES accounts (tenant, id)
);

CREATE INDEX page_secrets_of_a_page ON page_secrets (tenant, page);

-- Every value a secret had, the newest the one in use. origin says where a version came from, such as a restore.
CREATE TABLE page_secret_versions (
    tenant text NOT NULL DEFAULT current_setting('underdoc.tenant', true) REFERENCES tenants (id),
    secret text NOT NULL,
    number integer NOT NULL CHECK (number >= 1),
    sealed bytea NOT NULL,
    author text NOT NULL,
    at     timestamptz NOT NULL DEFAULT now(),
    origin text NOT NULL DEFAULT '' CHECK (length(origin) <= 200),
    PRIMARY KEY (tenant, secret, number),
    FOREIGN KEY (tenant, secret) REFERENCES page_secrets (tenant, id) ON DELETE CASCADE,
    FOREIGN KEY (tenant, author) REFERENCES accounts (tenant, id)
);

-- Who reveals a restricted secret: persons and groups. A group that goes takes its rows with it (DeleteGroup).
CREATE TABLE page_secret_readers (
    tenant text NOT NULL DEFAULT current_setting('underdoc.tenant', true) REFERENCES tenants (id),
    secret text NOT NULL,
    kind   text NOT NULL CHECK (kind IN ('person', 'group')),
    who    text NOT NULL,
    PRIMARY KEY (tenant, secret, kind, who),
    FOREIGN KEY (tenant, secret) REFERENCES page_secrets (tenant, id) ON DELETE CASCADE
);

ALTER TABLE page_secrets ENABLE ROW LEVEL SECURITY;
ALTER TABLE page_secrets FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_only ON page_secrets
    USING (tenant = current_setting('underdoc.tenant', true)) WITH CHECK (tenant = current_setting('underdoc.tenant', true));

ALTER TABLE page_secret_versions ENABLE ROW LEVEL SECURITY;
ALTER TABLE page_secret_versions FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_only ON page_secret_versions
    USING (tenant = current_setting('underdoc.tenant', true)) WITH CHECK (tenant = current_setting('underdoc.tenant', true));

ALTER TABLE page_secret_readers ENABLE ROW LEVEL SECURITY;
ALTER TABLE page_secret_readers FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_only ON page_secret_readers
    USING (tenant = current_setting('underdoc.tenant', true)) WITH CHECK (tenant = current_setting('underdoc.tenant', true));

-- Whether an account may reveal a secret: its page is alive, the account reads the page through its own roles (a role a
-- share link lends does not count), and, where the secret is restricted, the account or one of its groups is a reader.
-- The one check for secrets, as object_role is for objects.
CREATE FUNCTION secret_may(sec text, viewer text) RETURNS boolean LANGUAGE sql STABLE AS $$
    SELECT EXISTS (
        SELECT 1 FROM page_secrets s JOIN objects o ON o.id = s.page
        WHERE s.id = sec AND o.deleted_at IS NULL AND object_role(s.page, viewer, false) >= 1
          AND (NOT s.restricted OR EXISTS (
              SELECT 1 FROM page_secret_readers r WHERE r.secret = s.id
                AND ((r.kind = 'person' AND r.who = viewer) OR (r.kind = 'group' AND r.who = ANY (account_groups(viewer))))))
    )
$$;
