-- SPDX-License-Identifier: MIT
-- Files that are files (ADR-28): the database keeps the file object with its ID, place, owner, tags, comments and versions;
-- the content store keeps every version as an immutable blob named by its SHA-256, so equal content is stored once.
--
-- Every table carries its tenant, kept apart by row-level security as in migration 013 (ADR-23); a blob is a blob of
-- one tenant, so equal content of two tenants is two blobs.
--
-- The view Files has roots of its own (ADR-28, addendum of 2026-11-17): the folder of the tenant, a folder per user and
-- collections. They are collections like those of the documents, in an area of their own, so the documents never show
-- them and a file never lands among the pages.

ALTER TABLE objects ADD COLUMN area text NOT NULL DEFAULT 'docs' CHECK (area IN ('docs', 'files'));
ALTER TABLE objects DROP CONSTRAINT objects_kind_check;
ALTER TABLE objects ADD CONSTRAINT objects_kind_check CHECK (kind IN ('collection', 'folder', 'page', 'line', 'space', 'notes', 'file'));
-- A page lives among the documents, a file among the files.
ALTER TABLE objects ADD CONSTRAINT objects_kind_area_check CHECK ((kind <> 'page' OR area = 'docs') AND (kind <> 'file' OR area = 'files'));
-- A page counts its attachments like its blocks, so an attachment ID is never given out twice in a page.
ALTER TABLE objects ADD COLUMN next_attachment integer NOT NULL DEFAULT 1;

-- A root of the view Files: the folder of the tenant (all read, the group in writers writes), the folder of one user
-- (only that user), or a collection. Quota is in bytes, NULL for none of its own.
CREATE TABLE file_roots (
    tenant  text NOT NULL DEFAULT current_setting('underdoc.tenant', true) REFERENCES tenants (id),
    object  text NOT NULL,
    role    text NOT NULL CHECK (role IN ('tenant', 'user', 'collection')),
    account text,
    writers text NOT NULL DEFAULT '',
    quota   bigint CHECK (quota IS NULL OR quota >= 0),
    PRIMARY KEY (tenant, object),
    FOREIGN KEY (tenant, object) REFERENCES objects (tenant, id) ON DELETE CASCADE,
    FOREIGN KEY (tenant, account) REFERENCES accounts (tenant, id),
    CHECK ((role = 'user') = (account IS NOT NULL))
);

CREATE UNIQUE INDEX file_roots_one_tenant ON file_roots (tenant, role) WHERE role = 'tenant';
CREATE UNIQUE INDEX file_roots_one_per_user ON file_roots (tenant, account) WHERE role = 'user';

-- A collection of the documents can name a root of the files as "files of this space".
CREATE TABLE space_files (
    tenant text NOT NULL DEFAULT current_setting('underdoc.tenant', true) REFERENCES tenants (id),
    space  text NOT NULL,
    root   text NOT NULL,
    PRIMARY KEY (tenant, space),
    FOREIGN KEY (tenant, space) REFERENCES objects (tenant, id) ON DELETE CASCADE,
    FOREIGN KEY (tenant, root) REFERENCES objects (tenant, id) ON DELETE CASCADE
);

-- A blob of the content store. The row exists while something refers to it; the store is cleaned of blobs without a row.
CREATE TABLE blobs (
    tenant  text NOT NULL DEFAULT current_setting('underdoc.tenant', true) REFERENCES tenants (id),
    sha256  text NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    size    bigint NOT NULL CHECK (size >= 0),
    created timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant, sha256)
);

-- Every version of a file, with who made it and when (ADR-11). The newest is the file as it is now.
CREATE TABLE file_versions (
    tenant       text NOT NULL DEFAULT current_setting('underdoc.tenant', true) REFERENCES tenants (id),
    id           bigint GENERATED ALWAYS AS IDENTITY,
    file         text NOT NULL,
    number       integer NOT NULL CHECK (number >= 1),
    blob         text NOT NULL,
    size         bigint NOT NULL CHECK (size >= 0),
    name         text NOT NULL,
    content_type text NOT NULL,
    author       text NOT NULL,
    at           timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant, id),
    UNIQUE (tenant, file, number),
    FOREIGN KEY (tenant, file) REFERENCES objects (tenant, id) ON DELETE CASCADE,
    FOREIGN KEY (tenant, blob) REFERENCES blobs (tenant, sha256),
    FOREIGN KEY (tenant, author) REFERENCES accounts (tenant, id)
);

CREATE INDEX file_versions_by_blob ON file_versions (tenant, blob);

-- Attachments of a page, such as pasted images (ADR-28): they belong to the page like its blocks and live only in the store.
CREATE TABLE attachments (
    tenant       text NOT NULL DEFAULT current_setting('underdoc.tenant', true) REFERENCES tenants (id),
    page         text NOT NULL,
    id           text NOT NULL CHECK (id ~ '^a[1-9][0-9]{0,8}$'),
    blob         text NOT NULL,
    size         bigint NOT NULL CHECK (size >= 0),
    name         text NOT NULL,
    content_type text NOT NULL,
    author       text NOT NULL,
    at           timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant, page, id),
    FOREIGN KEY (tenant, page) REFERENCES objects (tenant, id) ON DELETE CASCADE,
    FOREIGN KEY (tenant, blob) REFERENCES blobs (tenant, sha256),
    FOREIGN KEY (tenant, author) REFERENCES accounts (tenant, id)
);

CREATE INDEX attachments_by_blob ON attachments (tenant, blob);

-- An upload in chunks that can resume. Its declared size counts against the quota from the start, so many uploads at once
-- cannot pass it. Parent is the folder for a new file, file the file for a new version, page the page for an attachment.
CREATE TABLE uploads (
    tenant   text NOT NULL DEFAULT current_setting('underdoc.tenant', true) REFERENCES tenants (id),
    id       text NOT NULL CHECK (id ~ '^[0-9a-f]{32}$'),
    owner    text NOT NULL,
    parent   text,
    file     text,
    page     text,
    root     text,
    name     text NOT NULL,
    size     bigint NOT NULL CHECK (size >= 0),
    received bigint NOT NULL DEFAULT 0 CHECK (received >= 0 AND received <= size),
    created  timestamptz NOT NULL DEFAULT now(),
    touched  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant, id),
    FOREIGN KEY (tenant, owner) REFERENCES accounts (tenant, id),
    FOREIGN KEY (tenant, parent) REFERENCES objects (tenant, id) ON DELETE CASCADE,
    FOREIGN KEY (tenant, file) REFERENCES objects (tenant, id) ON DELETE CASCADE,
    FOREIGN KEY (tenant, page) REFERENCES objects (tenant, id) ON DELETE CASCADE,
    FOREIGN KEY (tenant, root) REFERENCES objects (tenant, id) ON DELETE CASCADE,
    CHECK (num_nonnulls(parent, file, page) = 1)
);

CREATE INDEX uploads_by_root ON uploads (tenant, root);

-- Each tenant sees only its own rows (ADR-23, migration 013).
DO $$
DECLARE t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['file_roots', 'space_files', 'blobs', 'file_versions', 'attachments', 'uploads'] LOOP
        EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
        EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', t);
        EXECUTE format('CREATE POLICY tenant_only ON %I USING (tenant = current_setting(''underdoc.tenant'', true))
            WITH CHECK (tenant = current_setting(''underdoc.tenant'', true))', t);
    END LOOP;
END $$;
