-- SPDX-License-Identifier: MIT
-- Sign-in only through the gate (ADR-32). A person is bound by the lasting ID the gate gives the account (Remote-Id, its
-- token's ts_id), never by a name or a mail address (ADR-35). Until now the account of the gate was keyed by that ID,
-- or by its login where the gate sent none. The ID now has a column of its own, so that an account made before (a gate
-- account under its login, a local or an OIDC account) can be bound to its person in the gate without changing its key,
-- which every table refers to.
--
-- gate_kind keeps the kind of account the gate passed on last ("user" or "service"; empty: none), as it came.

ALTER TABLE accounts ADD COLUMN remote_id text CHECK (remote_id ~ '^[0-9]+$');
ALTER TABLE accounts ADD COLUMN gate_kind text NOT NULL DEFAULT '' CHECK (gate_kind IN ('', 'user', 'service'));
CREATE UNIQUE INDEX accounts_remote_id ON accounts (tenant, remote_id) WHERE remote_id IS NOT NULL;

-- An account of the gate whose key is a number was made from the ID the gate sent (the login was taken only without
-- one): it is bound to that ID. Every other account waits for `underdoc account bind`.
ALTER TABLE accounts NO FORCE ROW LEVEL SECURITY;
ALTER TABLE accounts DISABLE TRIGGER accounts_to_search;
UPDATE accounts SET remote_id = id WHERE source = 'gate' AND id ~ '^[0-9]+$';
ALTER TABLE accounts ENABLE TRIGGER accounts_to_search;
ALTER TABLE accounts FORCE ROW LEVEL SECURITY;
