-- SPDX-License-Identifier: MIT
-- Tenants (ADR-23). Every table carries its tenant, and row-level security keeps each tenant to its own rows: the app
-- sets the tenant of a request in the setting underdoc.tenant, and a query without it sees nothing and writes nothing.
-- What exists moves into the tenant Main, which is all there is in the mode one tenant.
--
-- Keys and references carry the tenant too, so no row can point at a row of another tenant, also not by a foreign key
-- that runs past row-level security. FORCE applies the rules to the owner of the tables as well. A later migration
-- that changes rows of a table turns FORCE off for itself and on again in the same transaction.
--
-- Only tenants and tenant_hosts are shared: they say which tenants exist and which host names lead to them, and hold
-- no content. The guard test in package db fails on any other table without tenant and rules.

CREATE TABLE tenants (
    id      text PRIMARY KEY CHECK (id ~ '^[a-z0-9]([a-z0-9-]{0,38}[a-z0-9])?$'),
    name    text NOT NULL CHECK (length(name) BETWEEN 1 AND 200),
    -- locked: nobody of the tenant gets in, nothing is lost. archived: deleted by the hoster, kept until purged.
    state   text NOT NULL DEFAULT 'active' CHECK (state IN ('active', 'locked', 'archived')),
    created timestamptz NOT NULL DEFAULT now(),
    changed timestamptz NOT NULL DEFAULT now()
);

INSERT INTO tenants (id, name) VALUES ('main', 'Main'), ('hoster', 'Hoster');

-- A domain of its own for a tenant, next to the subdomain of the hoster. A host leads to one tenant at most.
CREATE TABLE tenant_hosts (
    host   text PRIMARY KEY CHECK (host = lower(host) AND length(host) BETWEEN 1 AND 253),
    tenant text NOT NULL REFERENCES tenants (id) ON DELETE CASCADE
);

-- The tenant of every row. The default fills it from the setting, so no insert can name another one.
ALTER TABLE accounts       ADD COLUMN tenant text NOT NULL DEFAULT 'main' REFERENCES tenants (id);
ALTER TABLE objects        ADD COLUMN tenant text NOT NULL DEFAULT 'main' REFERENCES tenants (id);
ALTER TABLE blocks         ADD COLUMN tenant text NOT NULL DEFAULT 'main' REFERENCES tenants (id);
ALTER TABLE block_versions ADD COLUMN tenant text NOT NULL DEFAULT 'main' REFERENCES tenants (id);
ALTER TABLE refs           ADD COLUMN tenant text NOT NULL DEFAULT 'main' REFERENCES tenants (id);
ALTER TABLE comments       ADD COLUMN tenant text NOT NULL DEFAULT 'main' REFERENCES tenants (id);

ALTER TABLE accounts       ALTER COLUMN tenant SET DEFAULT current_setting('underdoc.tenant', true);
ALTER TABLE objects        ALTER COLUMN tenant SET DEFAULT current_setting('underdoc.tenant', true);
ALTER TABLE blocks         ALTER COLUMN tenant SET DEFAULT current_setting('underdoc.tenant', true);
ALTER TABLE block_versions ALTER COLUMN tenant SET DEFAULT current_setting('underdoc.tenant', true);
ALTER TABLE refs           ALTER COLUMN tenant SET DEFAULT current_setting('underdoc.tenant', true);
ALTER TABLE comments       ALTER COLUMN tenant SET DEFAULT current_setting('underdoc.tenant', true);

-- Keys with the tenant in front. An ID is unique within its tenant, so an import never collides with another tenant.
-- Numbers drawn from a sequence stay unique across tenants all the same.
ALTER TABLE objects        DROP CONSTRAINT objects_deleted_by_fkey, DROP CONSTRAINT objects_owner_fkey, DROP CONSTRAINT objects_parent_fkey;
ALTER TABLE blocks         DROP CONSTRAINT blocks_page_fkey;
ALTER TABLE block_versions DROP CONSTRAINT block_versions_page_fkey, DROP CONSTRAINT block_versions_author_fkey;
ALTER TABLE refs           DROP CONSTRAINT refs_page_block_fkey;
ALTER TABLE comments       DROP CONSTRAINT comments_resolved_by_fkey, DROP CONSTRAINT comments_parent_fkey,
                           DROP CONSTRAINT comments_author_fkey, DROP CONSTRAINT comments_page_fkey;

ALTER TABLE accounts DROP CONSTRAINT accounts_pkey, ADD PRIMARY KEY (tenant, id);
ALTER TABLE objects  DROP CONSTRAINT objects_pkey,  ADD PRIMARY KEY (tenant, id);
ALTER TABLE blocks   DROP CONSTRAINT blocks_pkey,   ADD PRIMARY KEY (tenant, page, id);
ALTER TABLE refs     DROP CONSTRAINT refs_pkey,     ADD PRIMARY KEY (tenant, page, block, target, target_block);
ALTER TABLE block_versions DROP CONSTRAINT block_versions_pkey, ADD PRIMARY KEY (tenant, version);
ALTER TABLE comments DROP CONSTRAINT comments_pkey, ADD PRIMARY KEY (tenant, id);

ALTER TABLE objects
    ADD FOREIGN KEY (tenant, owner) REFERENCES accounts (tenant, id),
    ADD FOREIGN KEY (tenant, deleted_by) REFERENCES accounts (tenant, id),
    ADD FOREIGN KEY (tenant, parent) REFERENCES objects (tenant, id) ON DELETE RESTRICT;
ALTER TABLE blocks
    ADD FOREIGN KEY (tenant, page) REFERENCES objects (tenant, id) ON DELETE CASCADE;
ALTER TABLE block_versions
    ADD FOREIGN KEY (tenant, page) REFERENCES objects (tenant, id) ON DELETE CASCADE,
    ADD FOREIGN KEY (tenant, author) REFERENCES accounts (tenant, id);
ALTER TABLE refs
    ADD FOREIGN KEY (tenant, page, block) REFERENCES blocks (tenant, page, id) ON DELETE CASCADE;
ALTER TABLE comments
    ADD FOREIGN KEY (tenant, page) REFERENCES objects (tenant, id) ON DELETE CASCADE,
    ADD FOREIGN KEY (tenant, parent) REFERENCES comments (tenant, id) ON DELETE CASCADE,
    ADD FOREIGN KEY (tenant, author) REFERENCES accounts (tenant, id),
    ADD FOREIGN KEY (tenant, resolved_by) REFERENCES accounts (tenant, id);

-- Indexes with the tenant in front, so one tenant's lookups do not walk the rows of all others.
DROP INDEX objects_by_parent;
CREATE INDEX objects_by_parent ON objects (tenant, parent, position, created);
DROP INDEX objects_at_the_top;
CREATE INDEX objects_at_the_top ON objects (tenant, position, created) WHERE parent IS NULL;
DROP INDEX block_versions_of_page;
CREATE INDEX block_versions_of_page ON block_versions (tenant, page, version DESC);
DROP INDEX refs_to_target;
CREATE INDEX refs_to_target ON refs (tenant, target, target_block);
DROP INDEX comments_of_page;
CREATE INDEX comments_of_page ON comments (tenant, page, block);

-- The rules. One per table, the same for reading and writing.
ALTER TABLE accounts ENABLE ROW LEVEL SECURITY;
ALTER TABLE accounts FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_only ON accounts
    USING (tenant = current_setting('underdoc.tenant', true)) WITH CHECK (tenant = current_setting('underdoc.tenant', true));

ALTER TABLE objects ENABLE ROW LEVEL SECURITY;
ALTER TABLE objects FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_only ON objects
    USING (tenant = current_setting('underdoc.tenant', true)) WITH CHECK (tenant = current_setting('underdoc.tenant', true));

ALTER TABLE blocks ENABLE ROW LEVEL SECURITY;
ALTER TABLE blocks FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_only ON blocks
    USING (tenant = current_setting('underdoc.tenant', true)) WITH CHECK (tenant = current_setting('underdoc.tenant', true));

ALTER TABLE block_versions ENABLE ROW LEVEL SECURITY;
ALTER TABLE block_versions FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_only ON block_versions
    USING (tenant = current_setting('underdoc.tenant', true)) WITH CHECK (tenant = current_setting('underdoc.tenant', true));

ALTER TABLE refs ENABLE ROW LEVEL SECURITY;
ALTER TABLE refs FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_only ON refs
    USING (tenant = current_setting('underdoc.tenant', true)) WITH CHECK (tenant = current_setting('underdoc.tenant', true));

ALTER TABLE comments ENABLE ROW LEVEL SECURITY;
ALTER TABLE comments FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_only ON comments
    USING (tenant = current_setting('underdoc.tenant', true)) WITH CHECK (tenant = current_setting('underdoc.tenant', true));
