-- SPDX-License-Identifier: MIT
-- What the hoster sets per tenant (ADR-23): limits on users and storage, and the modules it may use (ADR-25; NULL: all
-- that the instance allows). Then the choice of each tenant among those modules, and what each user hides.

ALTER TABLE tenants ADD COLUMN max_users   integer CHECK (max_users IS NULL OR max_users >= 0);
ALTER TABLE tenants ADD COLUMN max_storage bigint  CHECK (max_storage IS NULL OR max_storage >= 0);
ALTER TABLE tenants ADD COLUMN modules     text[];
ALTER TABLE tenants ADD COLUMN archived_at timestamptz;

-- The modules the administrator of a tenant switched on. No row: the defaults of the instance.
CREATE TABLE module_settings (
    tenant  text NOT NULL DEFAULT current_setting('underdoc.tenant', true) REFERENCES tenants (id),
    enabled text[] NOT NULL,
    changed timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant)
);

ALTER TABLE module_settings ENABLE ROW LEVEL SECURITY;
ALTER TABLE module_settings FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_only ON module_settings
    USING (tenant = current_setting('underdoc.tenant', true)) WITH CHECK (tenant = current_setting('underdoc.tenant', true));

-- The modules a user hides for themselves. Hiding is not switching off: what a hidden module holds stays reachable.
ALTER TABLE accounts ADD COLUMN hidden_modules text[] NOT NULL DEFAULT '{}';
