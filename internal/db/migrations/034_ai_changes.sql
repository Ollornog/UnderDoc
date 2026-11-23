-- SPDX-License-Identifier: MIT
-- Changes by agents are visible (ADR-33, T-20). Every version records whether an agent wrote it. The application never
-- says so itself: a trigger takes it from the connection, which the application sets from the checked sign-in of the
-- request (request_agent, migration 030), so no client and no code path can claim otherwise.
--
-- ai_changes holds, per block of human documentation, the change of agents nobody has checked yet: base is the version
-- before the first of them (NULL when an agent made the block), head the latest, which a check has to name, so it never
-- checks what it did not see. follows and precedes keep the blocks around one an agent deleted, for taking the deletion
-- back where it stood. A check or a person's own change of the block ends it.
--
-- checked_by, checked_name and checked_at keep the last check of a block: the account (empty for a check an import
-- brought), the name to show, and when.

ALTER TABLE block_versions ADD COLUMN agent boolean NOT NULL DEFAULT false;

CREATE FUNCTION block_version_agent() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'UPDATE' THEN
        NEW.agent := OLD.agent;
    ELSE
        NEW.agent := request_agent();
    END IF;
    RETURN NEW;
END
$$;

CREATE TRIGGER block_versions_agent BEFORE INSERT OR UPDATE ON block_versions
    FOR EACH ROW EXECUTE FUNCTION block_version_agent();

CREATE TABLE ai_changes (
    tenant      text NOT NULL DEFAULT current_setting('underdoc.tenant', true) REFERENCES tenants (id),
    page        text NOT NULL,
    block       text NOT NULL,
    base        bigint,
    head        bigint NOT NULL,
    follows     text NOT NULL DEFAULT '',
    precedes    text NOT NULL DEFAULT '',
    at          timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant, page, block),
    FOREIGN KEY (tenant, page) REFERENCES objects (tenant, id) ON DELETE CASCADE
);

ALTER TABLE ai_changes ENABLE ROW LEVEL SECURITY;
ALTER TABLE ai_changes FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_only ON ai_changes
    USING (tenant = current_setting('underdoc.tenant', true)) WITH CHECK (tenant = current_setting('underdoc.tenant', true));

ALTER TABLE blocks ADD COLUMN checked_by text NOT NULL DEFAULT '',
    ADD COLUMN checked_name text NOT NULL DEFAULT '' CHECK (length(checked_name) <= 200),
    ADD COLUMN checked_at timestamptz;
