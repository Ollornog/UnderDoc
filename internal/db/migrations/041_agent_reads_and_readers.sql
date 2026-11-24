-- SPDX-License-Identifier: MIT
-- An agent makes something readable to more people (T-38). It cannot be forbidden that an agent writes what it read
-- somewhere more people read; it is made visible instead.
--
-- agent_reads keeps, per account, what its agents read through MCP and when, for a while (AgentReadWindow): what the
-- agent may carry into its next write. A write to where more people read than read what it read in that time carries a
-- mark in the hint of changes by agents (ai_changes.wider), shown until a person checks it, also on pages of the agent
-- side. objects.agent_mark switches that mark per space (a collection at the top): NULL takes the default of the
-- instance (off with one person, on otherwise).
CREATE TABLE agent_reads (
    tenant  text NOT NULL DEFAULT current_setting('underdoc.tenant', true) REFERENCES tenants (id),
    account text NOT NULL,
    object  text NOT NULL,
    at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant, account, object),
    FOREIGN KEY (tenant, account) REFERENCES accounts (tenant, id) ON DELETE CASCADE,
    FOREIGN KEY (tenant, object) REFERENCES objects (tenant, id) ON DELETE CASCADE
);
CREATE INDEX agent_reads_at ON agent_reads (tenant, account, at);
ALTER TABLE agent_reads ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_reads FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_only ON agent_reads
    USING (tenant = current_setting('underdoc.tenant', true)) WITH CHECK (tenant = current_setting('underdoc.tenant', true));

ALTER TABLE ai_changes ADD COLUMN wider boolean NOT NULL DEFAULT false;
ALTER TABLE objects ADD COLUMN agent_mark boolean;
ALTER TABLE objects ADD CONSTRAINT objects_agent_mark_space CHECK (agent_mark IS NULL OR parent IS NULL);

-- The accounts that may read an object, links they took included: who reads what is written there.
CREATE FUNCTION object_readers(obj text) RETURNS text[] LANGUAGE sql STABLE AS $$
    SELECT coalesce(array_agg(a.id ORDER BY a.id), '{}') FROM accounts a WHERE (object_decide(obj, a.id, 'read', false, true)).allowed
$$;
