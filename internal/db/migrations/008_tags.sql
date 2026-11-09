-- SPDX-License-Identifier: MIT
-- Pages, folders and collections carry tags (M-28): short words to find things across places. They are kept with the
-- object, in the order given, lower case and without duplicates; the database takes at most 50 per object.

ALTER TABLE objects ADD COLUMN tags text[] NOT NULL DEFAULT '{}' CHECK (cardinality(tags) <= 50);

CREATE INDEX objects_by_tag ON objects USING gin (tags);
