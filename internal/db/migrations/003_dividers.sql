-- SPDX-License-Identifier: MIT
-- A line or a space sets entries of a collection apart (2026-11-08). It is an object like the others, so it has its place,
-- moves with them and is shared with everyone who sees the collection.

ALTER TABLE objects DROP CONSTRAINT objects_kind_check;
ALTER TABLE objects ADD CONSTRAINT objects_kind_check CHECK (kind IN ('collection', 'folder', 'page', 'line', 'space'));
