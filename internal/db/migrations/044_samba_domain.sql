-- SPDX-License-Identifier: MIT
-- Samba joins a domain, optionally (T-42). A tenant whose companion is a member of an Active Directory domain has Samba
-- users without an app password: the domain checks the password, and the companion maps the domain account of that name
-- to the UID kept here. So the NT hash may be missing, and the name is the name of the domain account, which may start
-- with a digit (a number from the gate, Remote-Id). The mode of a tenant is its setup (UNDERDOC_SAMBA_DOMAIN), never
-- mixed: UnderDoc refuses to start while a tenant holds Samba users of the other mode.
ALTER TABLE samba_accounts ALTER COLUMN nt_hash DROP NOT NULL;
ALTER TABLE samba_accounts DROP CONSTRAINT samba_accounts_name_check;
ALTER TABLE samba_accounts ADD CONSTRAINT samba_accounts_name_check CHECK (name ~ '^[a-z0-9][a-z0-9._-]{0,31}$');
