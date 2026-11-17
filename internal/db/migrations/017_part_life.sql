-- SPDX-License-Identifier: MIT
-- The end of something in use is chosen (ADR-22). removed keeps what was deleted everywhere, so every embedding of it shows
-- who removed it and when, and restoring it brings every use back. redirects keeps an old address alive after its content
-- moved to a new master. notices tells the owner of a page what was done or is to be chosen there; the inbox (M-16) shows
-- them. A version of a block may say where its text was copied from.

CREATE TABLE removed (
    tenant     text NOT NULL DEFAULT current_setting('underdoc.tenant', true) REFERENCES tenants (id),
    id         bigint GENERATED ALWAYS AS IDENTITY,
    page       text NOT NULL,
    -- b3, b3.t2 or b3.section: what went.
    target     text NOT NULL CHECK (target ~ '^b[1-9][0-9]{0,8}(\.(t[1-9][0-9]{0,8}|section))?$'),
    -- The blocks it took, [{"id", "body", "align"}]; for a part the one block with the part's Markdown and its markers.
    blocks     jsonb NOT NULL,
    -- The block before it, to put it back in its place.
    after      text NOT NULL DEFAULT '',
    removed_by text NOT NULL,
    removed_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant, id),
    FOREIGN KEY (tenant, page) REFERENCES objects (tenant, id) ON DELETE CASCADE,
    FOREIGN KEY (tenant, removed_by) REFERENCES accounts (tenant, id)
);

CREATE INDEX removed_of ON removed (tenant, page, target, removed_at DESC);

-- No key on the pages: the old one may be gone, the new one may go later; a redirect to nothing is a broken reference.
CREATE TABLE redirects (
    tenant      text NOT NULL DEFAULT current_setting('underdoc.tenant', true) REFERENCES tenants (id),
    from_page   text NOT NULL,
    from_target text NOT NULL,
    to_page     text NOT NULL,
    to_target   text NOT NULL,
    moved_by    text NOT NULL,
    moved_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant, from_page, from_target),
    FOREIGN KEY (tenant, moved_by) REFERENCES accounts (tenant, id)
);

CREATE TABLE notices (
    tenant  text NOT NULL DEFAULT current_setting('underdoc.tenant', true) REFERENCES tenants (id),
    id      bigint GENERATED ALWAYS AS IDENTITY,
    -- Who is told.
    account text NOT NULL,
    -- master: the content moved into a page of the account; copy: something its page embedded is gone, and a copy
    -- could not be written there.
    kind    text NOT NULL CHECK (kind IN ('master', 'copy')),
    page    text NOT NULL,
    block   text NOT NULL DEFAULT '',
    ref     text NOT NULL DEFAULT '',
    by      text NOT NULL,
    at      timestamptz NOT NULL DEFAULT now(),
    read_at timestamptz,
    PRIMARY KEY (tenant, id),
    FOREIGN KEY (tenant, account) REFERENCES accounts (tenant, id),
    FOREIGN KEY (tenant, by) REFERENCES accounts (tenant, id)
);

CREATE INDEX notices_of ON notices (tenant, account, at DESC);

ALTER TABLE block_versions ADD COLUMN origin text NOT NULL DEFAULT '';

ALTER TABLE removed ENABLE ROW LEVEL SECURITY;
ALTER TABLE removed FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_only ON removed
    USING (tenant = current_setting('underdoc.tenant', true)) WITH CHECK (tenant = current_setting('underdoc.tenant', true));

ALTER TABLE redirects ENABLE ROW LEVEL SECURITY;
ALTER TABLE redirects FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_only ON redirects
    USING (tenant = current_setting('underdoc.tenant', true)) WITH CHECK (tenant = current_setting('underdoc.tenant', true));

ALTER TABLE notices ENABLE ROW LEVEL SECURITY;
ALTER TABLE notices FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_only ON notices
    USING (tenant = current_setting('underdoc.tenant', true)) WITH CHECK (tenant = current_setting('underdoc.tenant', true));
