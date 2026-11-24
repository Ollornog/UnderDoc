-- SPDX-License-Identifier: MIT
-- The reads of an agent keep who read what was read, at that moment (B-7): a write is compared with that, in one query,
-- with no limit on how many reads count, and a read stays when its object goes to the trash or is deleted for good
-- (a read, a trash, a write for everyone would otherwise lose the mark). readers are the accounts that read it, links
-- tells that a live link showed it.
--
-- The window a read counts in is one working day per person (agent_read_hours, 12 hours unless the person sets it,
-- 1 to 168); checking a marked change in the browser ends the window of whoever checks it.
--
-- The reads kept so far have no readers: they are dropped (at most two hours of them; their marks would be guesses).
DELETE FROM agent_reads;
ALTER TABLE agent_reads DROP CONSTRAINT agent_reads_tenant_object_fkey;
ALTER TABLE agent_reads ADD COLUMN readers text[] NOT NULL DEFAULT '{}';
ALTER TABLE agent_reads ADD COLUMN links boolean NOT NULL DEFAULT false;
ALTER TABLE accounts ADD COLUMN agent_read_hours integer NOT NULL DEFAULT 12 CHECK (agent_read_hours BETWEEN 1 AND 168);
