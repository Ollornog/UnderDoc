-- SPDX-License-Identifier: MIT
-- One-time upload links (ADR-34, T-21): an agent never sends a file inside a message. It asks for a link to an upload it
-- started with its person's rights, and a program sends the file there once, within a few minutes. Only the hash of the
-- link is kept, and taking the link removes it, so it works once.

ALTER TABLE uploads ADD COLUMN link_hash bytea;
ALTER TABLE uploads ADD COLUMN link_until timestamptz;
CREATE UNIQUE INDEX uploads_link ON uploads (tenant, link_hash) WHERE link_hash IS NOT NULL;
