-- SPDX-License-Identifier: MIT
-- Quick notes are a document type of their own (ADR-16, M-8): small cards on a board, each card a block of the board,
-- so cards keep IDs, versions and comments as blocks of a page do. Every account's board lies in its private notes.

ALTER TABLE objects DROP CONSTRAINT objects_kind_check;
ALTER TABLE objects ADD CONSTRAINT objects_kind_check CHECK (kind IN ('collection', 'folder', 'page', 'line', 'space', 'notes'));
