-- SPDX-License-Identifier: MIT
-- The storage a tenant uses, as its hoster limits it (ADR-23): the bytes of the current blocks of its pages, its files with
-- every version, its attachments and its running uploads. Until now every write that could grow it summed all of them;
-- now triggers keep one counter per tenant, whichever way a row comes or goes, so a check reads one row. A write takes
-- the row of its tenant until it commits, so two writes that would each just fit cannot both pass the limit.

CREATE TABLE tenant_storage (
    tenant text NOT NULL DEFAULT current_setting('underdoc.tenant', true) REFERENCES tenants (id) ON DELETE CASCADE,
    bytes  bigint NOT NULL DEFAULT 0,
    PRIMARY KEY (tenant)
);

ALTER TABLE tenant_storage ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenant_storage FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_only ON tenant_storage
    USING (tenant = current_setting('underdoc.tenant', true)) WITH CHECK (tenant = current_setting('underdoc.tenant', true));

CREATE FUNCTION storage_add(of_tenant text, delta bigint) RETURNS void LANGUAGE sql AS $$
    INSERT INTO tenant_storage (tenant, bytes) VALUES (of_tenant, delta)
    ON CONFLICT (tenant) DO UPDATE SET bytes = tenant_storage.bytes + EXCLUDED.bytes
$$;

-- Blocks count with the bytes of their body; the other tables with their size.
CREATE FUNCTION storage_of_blocks() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        PERFORM storage_add(NEW.tenant, octet_length(NEW.body));
    ELSIF TG_OP = 'DELETE' THEN
        PERFORM storage_add(OLD.tenant, -octet_length(OLD.body));
    ELSIF octet_length(NEW.body) IS DISTINCT FROM octet_length(OLD.body) THEN
        PERFORM storage_add(NEW.tenant, octet_length(NEW.body) - octet_length(OLD.body));
    END IF;
    RETURN NULL;
END
$$;

CREATE FUNCTION storage_of_sizes() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        PERFORM storage_add(NEW.tenant, NEW.size);
    ELSIF TG_OP = 'DELETE' THEN
        PERFORM storage_add(OLD.tenant, -OLD.size);
    ELSIF NEW.size IS DISTINCT FROM OLD.size THEN
        PERFORM storage_add(NEW.tenant, NEW.size - OLD.size);
    END IF;
    RETURN NULL;
END
$$;

CREATE TRIGGER blocks_storage AFTER INSERT OR UPDATE OR DELETE ON blocks
    FOR EACH ROW EXECUTE FUNCTION storage_of_blocks();
CREATE TRIGGER file_versions_storage AFTER INSERT OR UPDATE OR DELETE ON file_versions
    FOR EACH ROW EXECUTE FUNCTION storage_of_sizes();
CREATE TRIGGER attachments_storage AFTER INSERT OR UPDATE OR DELETE ON attachments
    FOR EACH ROW EXECUTE FUNCTION storage_of_sizes();
CREATE TRIGGER uploads_storage AFTER INSERT OR UPDATE OR DELETE ON uploads
    FOR EACH ROW EXECUTE FUNCTION storage_of_sizes();

-- What every tenant uses today, counted once as that tenant (row-level security holds for the owner too).
DO $$
DECLARE t record;
BEGIN
    FOR t IN SELECT id FROM tenants LOOP
        PERFORM set_config('underdoc.tenant', t.id, true);
        INSERT INTO tenant_storage (tenant, bytes) VALUES (t.id,
            (SELECT coalesce(sum(octet_length(body)), 0) FROM blocks WHERE tenant = t.id)
            + (SELECT coalesce(sum(size), 0) FROM file_versions WHERE tenant = t.id)
            + (SELECT coalesce(sum(size), 0) FROM attachments WHERE tenant = t.id)
            + (SELECT coalesce(sum(size), 0) FROM uploads WHERE tenant = t.id));
    END LOOP;
    PERFORM set_config('underdoc.tenant', '', true);
END
$$;
