-- SPDX-License-Identifier: MIT
-- How long the log of access and changes (024) keeps an entry, per tenant (ADR-29, 2026-11-19): 365 days unless the
-- administrators of the tenant set between 30 and 3650. Without a row the tenant keeps the default; the job that empties
-- the trash deletes what is older (KeepTrash).

CREATE TABLE log_settings (
    tenant    text NOT NULL DEFAULT current_setting('underdoc.tenant', true) REFERENCES tenants (id),
    keep_days integer NOT NULL DEFAULT 365 CHECK (keep_days BETWEEN 30 AND 3650),
    changed   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant)
);

ALTER TABLE log_settings ENABLE ROW LEVEL SECURITY;
ALTER TABLE log_settings FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_only ON log_settings
    USING (tenant = current_setting('underdoc.tenant', true)) WITH CHECK (tenant = current_setting('underdoc.tenant', true));
