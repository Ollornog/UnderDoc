-- SPDX-License-Identifier: MIT
-- Sign-in only through the gate (ADR-32). The sign-in built in by ADR-23 goes: the ways in chosen per tenant, passkeys,
-- sessions, the steps between two requests of a passkey or OIDC, invitations and registration, the log of sign-ins, and
-- the passwords and TOTP secrets of local accounts. The gate keeps all of that now.
--
-- What stays of an account: its key, everything it owns, where it came from (source), and its mark as an administrator,
-- which is now the emergency access of the command line. An account made locally or by OIDC is bound to its person in
-- the gate with `underdoc account bind`; a local administrator stays marked, and administers again once bound.
--
-- The sealed secrets of the sign-in go with it: the client secret of OIDC. The TOTP secrets were columns of the accounts.

DROP TABLE sign_in_log;
DROP TABLE invites;
DROP TABLE ceremonies;
DROP TABLE sessions;
DROP TABLE passkeys;
DROP TABLE sign_in_settings;

DROP INDEX accounts_local_login;
ALTER TABLE accounts DROP COLUMN password_hash, DROP COLUMN totp_sealed, DROP COLUMN totp_step;

ALTER TABLE secrets NO FORCE ROW LEVEL SECURITY;
DELETE FROM secrets WHERE name = 'oidc.client_secret';
ALTER TABLE secrets FORCE ROW LEVEL SECURITY;
