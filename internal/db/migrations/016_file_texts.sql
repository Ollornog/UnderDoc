-- SPDX-License-Identifier: MIT
-- The text of files for the search (ADR-24): it belongs to the content, so equal content is read once, and each way that
-- read it keeps its own text, so the index can say where a text came from. Each way is switched on by the hoster, the
-- tenant and the user in turn; a queue with limits does the work on the server. Every table carries its tenant (ADR-23).

CREATE TABLE file_texts (
    tenant text NOT NULL DEFAULT current_setting('underdoc.tenant', true) REFERENCES tenants (id),
    blob   text NOT NULL,
    source text NOT NULL CHECK (source IN ('browser', 'server', 'ocr', 'ai')),
    text   text NOT NULL CHECK (octet_length(text) <= 1048576),
    author text,
    at     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant, blob, source),
    FOREIGN KEY (tenant, blob) REFERENCES blobs (tenant, sha256) ON DELETE CASCADE,
    FOREIGN KEY (tenant, author) REFERENCES accounts (tenant, id)
);

-- Work for the server: read the text of a blob one way. Account is whose upload it was, for the quota of a model.
CREATE TABLE text_jobs (
    tenant       text NOT NULL DEFAULT current_setting('underdoc.tenant', true) REFERENCES tenants (id),
    blob         text NOT NULL,
    way          text NOT NULL CHECK (way IN ('server', 'ocr', 'ai')),
    account      text NOT NULL,
    content_type text NOT NULL,
    tries        integer NOT NULL DEFAULT 0,
    due          timestamptz NOT NULL DEFAULT now(),
    failed       text NOT NULL DEFAULT '',
    PRIMARY KEY (tenant, blob, way),
    FOREIGN KEY (tenant, blob) REFERENCES blobs (tenant, sha256) ON DELETE CASCADE,
    FOREIGN KEY (tenant, account) REFERENCES accounts (tenant, id)
);

CREATE INDEX text_jobs_due ON text_jobs (tenant, due) WHERE tries < 3;

-- Which ways the tenant (account '') and each user switch on; NULL takes it from above.
CREATE TABLE text_settings (
    tenant  text NOT NULL DEFAULT current_setting('underdoc.tenant', true) REFERENCES tenants (id),
    account text NOT NULL,
    browser boolean,
    server  boolean,
    ai      boolean,
    PRIMARY KEY (tenant, account)
);

-- Pages a model for OCR read, per account and month, against the quotas of the user and the tenant.
CREATE TABLE ai_ocr_usage (
    tenant  text NOT NULL DEFAULT current_setting('underdoc.tenant', true) REFERENCES tenants (id),
    account text NOT NULL,
    month   date NOT NULL,
    pages   integer NOT NULL DEFAULT 0 CHECK (pages >= 0),
    PRIMARY KEY (tenant, account, month),
    FOREIGN KEY (tenant, account) REFERENCES accounts (tenant, id)
);

DO $$
DECLARE t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['file_texts', 'text_jobs', 'text_settings', 'ai_ocr_usage'] LOOP
        EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
        EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', t);
        EXECUTE format('CREATE POLICY tenant_only ON %I USING (tenant = current_setting(''underdoc.tenant'', true))
            WITH CHECK (tenant = current_setting(''underdoc.tenant'', true))', t);
    END LOOP;
END $$;
