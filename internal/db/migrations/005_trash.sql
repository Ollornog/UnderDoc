-- SPDX-License-Identifier: MIT
-- Deleting an object puts it into the trash with everything below it (2026-11-09). A deleted object keeps its row, its
-- place and its blocks, so it can be restored where it lay; deleted_at marks it, and the same moment marks everything
-- that went with it, so restoring brings back that batch and nothing deleted on its own before.

ALTER TABLE objects ADD COLUMN deleted_at timestamptz;
ALTER TABLE objects ADD COLUMN deleted_by text REFERENCES accounts (id);
ALTER TABLE objects ADD CONSTRAINT objects_deleted_check CHECK ((deleted_at IS NULL) = (deleted_by IS NULL));

CREATE INDEX objects_in_the_trash ON objects (deleted_at DESC) WHERE deleted_at IS NOT NULL;
