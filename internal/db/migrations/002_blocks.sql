-- SPDX-License-Identifier: MIT
-- A page is a list of blocks, each a piece of Markdown with an ID local to its page (ADR-18, ADR-19). Every change of a
-- block is kept as a version, with who made it and when (ADR-11). A block ID is never given out twice in a page, also
-- after the block is deleted: next_block counts on.

ALTER TABLE objects ADD COLUMN next_block integer NOT NULL DEFAULT 1;

CREATE TABLE blocks (
    page     text NOT NULL REFERENCES objects (id) ON DELETE CASCADE,
    id       text NOT NULL CHECK (id ~ '^b[1-9][0-9]{0,8}$'),
    position integer NOT NULL,
    body     text NOT NULL CHECK (length(body) <= 100000),
    PRIMARY KEY (page, id)
);

CREATE INDEX blocks_in_order ON blocks (page, position);

CREATE TABLE block_versions (
    page    text NOT NULL REFERENCES objects (id) ON DELETE CASCADE,
    block   text NOT NULL,
    version bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    -- NULL marks the deletion of the block.
    body    text,
    author  text NOT NULL REFERENCES accounts (id),
    at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX block_versions_of_page ON block_versions (page, version DESC);
