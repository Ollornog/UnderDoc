-- SPDX-License-Identifier: MIT
-- Comments on a page, on the whole page (block '') or on one of its blocks through the block's ID (ADR-18, ADR-20). A
-- reply names the comment it answers; replies go one level deep, so a thread is a comment and its replies. A thread is
-- resolved as a whole, on its first comment.
--
-- The block is not a foreign key on purpose: a deleted block keeps its comments, which show as "on a block that is
-- gone". Block IDs are never given out twice in a page, so such a comment can never land on another block.

CREATE TABLE comments (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    page        text NOT NULL REFERENCES objects (id) ON DELETE CASCADE,
    block       text NOT NULL DEFAULT '' CHECK (block = '' OR block ~ '^b[1-9][0-9]{0,8}$'),
    parent      bigint REFERENCES comments (id) ON DELETE CASCADE,
    author      text NOT NULL REFERENCES accounts (id),
    -- Plain text, never rendered as Markdown or HTML.
    body        text NOT NULL CHECK (length(body) BETWEEN 1 AND 10000),
    created     timestamptz NOT NULL DEFAULT now(),
    edited      timestamptz,
    resolved_at timestamptz,
    resolved_by text REFERENCES accounts (id),
    CHECK ((resolved_at IS NULL) = (resolved_by IS NULL)),
    CHECK (parent IS NULL OR resolved_at IS NULL)
);

CREATE INDEX comments_of_page ON comments (page, block);
CREATE INDEX comments_replies ON comments (parent) WHERE parent IS NOT NULL;
