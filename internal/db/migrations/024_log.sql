-- SPDX-License-Identifier: MIT
-- The log of access and changes (ADR-29, M-5): who opened or changed what, and every change of rights, groups,
-- administrators and links. The actor is an account, or empty for someone without one who came through a link. The
-- object stays a plain ID, so an entry outlives what it names.

CREATE TABLE audit_log (
    tenant text NOT NULL DEFAULT current_setting('underdoc.tenant', true) REFERENCES tenants (id),
    id     bigint GENERATED ALWAYS AS IDENTITY,
    at     timestamptz NOT NULL DEFAULT now(),
    actor  text NOT NULL DEFAULT '',
    action text NOT NULL CHECK (length(action) BETWEEN 1 AND 40),
    object text NOT NULL DEFAULT '',
    detail text NOT NULL DEFAULT '' CHECK (length(detail) <= 1000),
    PRIMARY KEY (tenant, id)
);

CREATE INDEX audit_log_by_time ON audit_log (tenant, at DESC);
CREATE INDEX audit_log_by_object ON audit_log (tenant, object, at DESC);
CREATE INDEX audit_log_by_actor ON audit_log (tenant, actor, action, object, at DESC);

ALTER TABLE audit_log ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_log FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_only ON audit_log
    USING (tenant = current_setting('underdoc.tenant', true)) WITH CHECK (tenant = current_setting('underdoc.tenant', true));

-- Whether an object lies in something private: its log shows to its owner alone, administrators included.
CREATE FUNCTION object_private(obj text) RETURNS boolean LANGUAGE sql STABLE AS $$
    WITH RECURSIVE up AS (
        SELECT id, parent, private FROM objects WHERE id = obj
        UNION ALL SELECT p.id, p.parent, p.private FROM objects p JOIN up ON p.id = up.parent
    )
    SELECT coalesce(bool_or(private), false) FROM up
$$;
