-- SPDX-License-Identifier: MIT
-- The settings of the workspace belong to the account, so they follow it from browser to browser (M-28): the order of
-- nav2 with its lines, spaces and the collections shown there, what is kept open, widths, zoom and theme. The app owns
-- their shape; the database keeps them as one JSON object.

ALTER TABLE accounts ADD COLUMN settings jsonb NOT NULL DEFAULT '{}'::jsonb;
