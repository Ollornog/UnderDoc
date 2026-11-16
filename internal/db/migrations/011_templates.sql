-- SPDX-License-Identifier: MIT
-- A page can be offered as a template (M-8): New starts a page as a copy of its blocks. Templates of UnderDoc itself live
-- in the code; the ones people make are pages, kept where they were written, with their owner and access.

ALTER TABLE objects ADD COLUMN template boolean NOT NULL DEFAULT false;

CREATE INDEX objects_templates ON objects (title) WHERE template;
