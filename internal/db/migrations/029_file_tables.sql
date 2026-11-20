-- SPDX-License-Identifier: MIT
-- Tables of spreadsheet files (ADR-31): a sheet, a range, a defined name or an Excel table that something uses gets an
-- ID local to its file (x1, x2, …), addressed as <file>#x3. UnderDoc keeps here what it points at, since the file itself
-- is never changed for it, and where it was last found, to follow it through new versions or to say that it is lost.

CREATE TABLE file_tables (
    tenant       text NOT NULL DEFAULT current_setting('underdoc.tenant', true) REFERENCES tenants (id),
    file         text NOT NULL,
    id           text NOT NULL CHECK (id ~ '^x[1-9][0-9]{0,8}$'),
    kind         text NOT NULL CHECK (kind IN ('sheet', 'range', 'name', 'table')),
    -- The sheet by its name, and its internal number (sheetId) where the file has one.
    sheet        text NOT NULL CHECK (length(sheet) <= 1024),
    sheet_number integer NOT NULL DEFAULT 0,
    -- The range of a range, such as A1:D20; for the others where it was last found.
    range        text NOT NULL DEFAULT '' CHECK (length(range) <= 40),
    -- The defined name or the table, by its name.
    name         text NOT NULL DEFAULT '' CHECK (length(name) <= 1024),
    -- head: the first row shows as the head of the table.
    head         boolean NOT NULL DEFAULT false,
    -- The values of the first row of a range when it was chosen, to find it again.
    anchor       text[] NOT NULL DEFAULT '{}',
    -- The blob it was last looked for in, the program that saved that blob, and what came of it: the range it moved from,
    -- whether its first row changed and it could not be followed, or why it is lost.
    found_blob   text NOT NULL DEFAULT '',
    found_app    text NOT NULL DEFAULT '',
    moved_from   text NOT NULL DEFAULT '' CHECK (length(moved_from) <= 1100),
    changed      boolean NOT NULL DEFAULT false,
    lost         text NOT NULL DEFAULT '' CHECK (length(lost) <= 2000),
    created_by   text NOT NULL,
    created      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant, file, id),
    FOREIGN KEY (tenant, file) REFERENCES objects (tenant, id) ON DELETE CASCADE,
    FOREIGN KEY (tenant, created_by) REFERENCES accounts (tenant, id)
);

ALTER TABLE file_tables ENABLE ROW LEVEL SECURITY;
ALTER TABLE file_tables FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_only ON file_tables
    USING (tenant = current_setting('underdoc.tenant', true)) WITH CHECK (tenant = current_setting('underdoc.tenant', true));
