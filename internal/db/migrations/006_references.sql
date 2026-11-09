-- SPDX-License-Identifier: MIT
-- A block can embed a block or a whole page of another page by its address (ADR-18): one source, shown live where it is
-- embedded. Every such reference is kept here in both directions, so each page and block can say where it is used. The
-- target is not a foreign key: a reference to something deleted or never there stays, and shows as broken.

CREATE TABLE refs (
    page         text NOT NULL,
    block        text NOT NULL,
    target       text NOT NULL,
    target_block text NOT NULL DEFAULT '',
    PRIMARY KEY (page, block, target, target_block),
    FOREIGN KEY (page, block) REFERENCES blocks (page, id) ON DELETE CASCADE
);

CREATE INDEX refs_to_target ON refs (target, target_block);
