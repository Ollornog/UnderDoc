-- SPDX-License-Identifier: MIT
-- A block of a page can be aligned center or right (ADR-16, M-8); NULL is the left, as text reads. The alignment belongs to
-- the block, not to its Markdown, so the text and its versions stay as written.

ALTER TABLE blocks ADD COLUMN align text CHECK (align IN ('center', 'right'));
