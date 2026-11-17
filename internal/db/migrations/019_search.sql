-- SPDX-License-Identifier: MIT
-- The search index is fed from here (ADR-24): every change of an object or a block puts the object into a queue, and the
-- indexer takes it from there to the search service. The queue lives in the database, so nothing is lost while the
-- search service is down, and the index can be thrown away and built again at any time.
--
-- A key is due at `due`. A change makes it due at once; a failed attempt puts it back for later. Each tenant has its own
-- queue and index (ADR-23): both tables carry the tenant and keep to it as the tables of 013_tenants.sql do.

CREATE TABLE search_queue (
    tenant text NOT NULL DEFAULT current_setting('underdoc.tenant', true) REFERENCES tenants (id),
    source text NOT NULL,
    key    text NOT NULL,
    due    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant, source, key)
);

CREATE INDEX search_queue_by_due ON search_queue (tenant, due);

-- The version of the settings each index was last built with: a new version builds it anew.
CREATE TABLE search_indexes (
    tenant  text NOT NULL DEFAULT current_setting('underdoc.tenant', true) REFERENCES tenants (id),
    name    text NOT NULL,
    version text NOT NULL,
    built   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant, name)
);

ALTER TABLE search_queue ENABLE ROW LEVEL SECURITY;
ALTER TABLE search_queue FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_only ON search_queue
    USING (tenant = current_setting('underdoc.tenant', true)) WITH CHECK (tenant = current_setting('underdoc.tenant', true));

ALTER TABLE search_indexes ENABLE ROW LEVEL SECURITY;
ALTER TABLE search_indexes FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_only ON search_indexes
    USING (tenant = current_setting('underdoc.tenant', true)) WITH CHECK (tenant = current_setting('underdoc.tenant', true));

CREATE FUNCTION search_enqueue(of_tenant text, keys text[]) RETURNS void LANGUAGE sql AS $$
    INSERT INTO search_queue (tenant, source, key) SELECT of_tenant, 'object', k FROM unnest(keys) AS k
    ON CONFLICT (tenant, source, key) DO UPDATE SET due = least(search_queue.due, now())
$$;

-- A change of title, place, privacy or owner shows in everything below the object as well: in its path and in who may
-- read it. Any other change only touches the object itself.
CREATE FUNCTION search_queue_object() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        PERFORM search_enqueue(OLD.tenant, ARRAY[OLD.id]);
        RETURN OLD;
    END IF;
    IF TG_OP = 'UPDATE' AND (NEW.title, NEW.parent, NEW.private, NEW.owner) IS NOT DISTINCT FROM
            (OLD.title, OLD.parent, OLD.private, OLD.owner) THEN
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

CREATE TRIGGER objects_to_search AFTER INSERT OR UPDATE OR DELETE ON objects
    FOR EACH ROW EXECUTE FUNCTION search_queue_object();

CREATE FUNCTION search_queue_block() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        PERFORM search_enqueue(OLD.tenant, ARRAY[OLD.page]);
        RETURN OLD;
    END IF;
    PERFORM search_enqueue(NEW.tenant, ARRAY[NEW.page]);
    RETURN NEW;
END
$$;

CREATE TRIGGER blocks_to_search AFTER INSERT OR UPDATE OR DELETE ON blocks
    FOR EACH ROW EXECUTE FUNCTION search_queue_block();

-- The name of an account stands in the records of what it owns.
CREATE FUNCTION search_queue_account() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.name IS DISTINCT FROM OLD.name THEN
        PERFORM search_enqueue(NEW.tenant, ARRAY(SELECT id FROM objects WHERE tenant = NEW.tenant AND owner = NEW.id));
    END IF;
    RETURN NEW;
END
$$;

CREATE TRIGGER accounts_to_search AFTER UPDATE ON accounts
    FOR EACH ROW EXECUTE FUNCTION search_queue_account();
