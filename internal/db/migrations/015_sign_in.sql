-- SPDX-License-Identifier: MIT
-- Sign-in built in (ADR-23): each tenant chooses its ways in, local accounts with a passkey or a password, OIDC, or the
-- gate in front. Every table carries the tenant and its rules, as 013 set out.

-- What follows reads accounts, which row-level security keeps to a tenant also for the owner of the tables.
SELECT set_config('underdoc.tenant', 'main', true);

CREATE TABLE sign_in_settings (
    tenant            text NOT NULL DEFAULT current_setting('underdoc.tenant', true) REFERENCES tenants (id),
    -- passkey, password_totp, password, oidc, gate; at least one.
    methods           text[] NOT NULL CHECK (cardinality(methods) BETWEEN 1 AND 5
                          AND methods <@ ARRAY['passkey', 'password_totp', 'password', 'oidc', 'gate']),
    registration      boolean NOT NULL DEFAULT false,
    oidc_name         text NOT NULL DEFAULT '' CHECK (length(oidc_name) <= 100),
    oidc_issuer       text NOT NULL DEFAULT '' CHECK (length(oidc_issuer) <= 500),
    oidc_client_id    text NOT NULL DEFAULT '' CHECK (length(oidc_client_id) <= 500),
    oidc_groups_claim text NOT NULL DEFAULT 'groups' CHECK (length(oidc_groups_claim) <= 100),
    -- The group whose members administer the tenant; empty means the default of the instance.
    admin_group       text NOT NULL DEFAULT '' CHECK (length(admin_group) <= 100),
    changed           timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant)
);

-- An instance that has accounts already came in through the gate, and keeps it. A new one starts with passkeys.
INSERT INTO sign_in_settings (tenant, methods)
SELECT 'main', CASE WHEN EXISTS (SELECT 1 FROM accounts) THEN ARRAY['gate'] ELSE ARRAY['passkey'] END;

-- Where an account comes from: the gate, OIDC, or a local account of this tenant with its own credentials.
ALTER TABLE accounts ADD COLUMN source text NOT NULL DEFAULT 'gate' CHECK (source IN ('gate', 'oidc', 'local'));
ALTER TABLE accounts ADD COLUMN password_hash text;
-- The TOTP secret, sealed by the vault under the key of the instance; totp_step is the last step used.
ALTER TABLE accounts ADD COLUMN totp_sealed bytea;
ALTER TABLE accounts ADD COLUMN totp_step bigint NOT NULL DEFAULT 0;
-- An administrator of the tenant among the local accounts; gate and OIDC tell it by the group.
ALTER TABLE accounts ADD COLUMN admin boolean NOT NULL DEFAULT false;
CREATE UNIQUE INDEX accounts_local_login ON accounts (tenant, lower(login)) WHERE source = 'local';

CREATE TABLE passkeys (
    tenant     text NOT NULL DEFAULT current_setting('underdoc.tenant', true) REFERENCES tenants (id),
    id         bytea NOT NULL CHECK (length(id) BETWEEN 1 AND 1023),
    account    text NOT NULL,
    name       text NOT NULL DEFAULT '' CHECK (length(name) <= 100),
    -- The credential as the WebAuthn library keeps it: public key, counter, flags, authenticator.
    credential jsonb NOT NULL,
    created    timestamptz NOT NULL DEFAULT now(),
    last_used  timestamptz,
    PRIMARY KEY (tenant, id),
    FOREIGN KEY (tenant, account) REFERENCES accounts (tenant, id) ON DELETE CASCADE
);

-- A session is the hash of the token in its cookie.
CREATE TABLE sessions (
    tenant  text NOT NULL DEFAULT current_setting('underdoc.tenant', true) REFERENCES tenants (id),
    hash    bytea NOT NULL CHECK (length(hash) = 32),
    account text NOT NULL,
    method  text NOT NULL,
    created timestamptz NOT NULL DEFAULT now(),
    seen    timestamptz NOT NULL DEFAULT now(),
    expires timestamptz NOT NULL,
    PRIMARY KEY (tenant, hash),
    FOREIGN KEY (tenant, account) REFERENCES accounts (tenant, id) ON DELETE CASCADE
);

-- A ceremony between two requests: the challenge of a passkey, or state, nonce and verifier of OIDC. It is used once.
CREATE TABLE ceremonies (
    tenant  text NOT NULL DEFAULT current_setting('underdoc.tenant', true) REFERENCES tenants (id),
    hash    bytea NOT NULL CHECK (length(hash) = 32),
    kind    text NOT NULL CHECK (kind IN ('passkey_sign_in', 'passkey_add', 'oidc')),
    account text,
    data    jsonb NOT NULL,
    expires timestamptz NOT NULL,
    PRIMARY KEY (tenant, hash),
    FOREIGN KEY (tenant, account) REFERENCES accounts (tenant, id) ON DELETE CASCADE
);

-- An invitation to a local account, by an administrator or by registration where that is on.
CREATE TABLE invites (
    tenant  text NOT NULL DEFAULT current_setting('underdoc.tenant', true) REFERENCES tenants (id),
    hash    bytea NOT NULL CHECK (length(hash) = 32),
    login   text NOT NULL CHECK (login ~ '^[a-z0-9][a-z0-9._-]{0,62}$'),
    name    text NOT NULL CHECK (length(name) BETWEEN 1 AND 200),
    admin   boolean NOT NULL DEFAULT false,
    -- Who invited; empty for a registration.
    made_by text NOT NULL DEFAULT '',
    created timestamptz NOT NULL DEFAULT now(),
    expires timestamptz NOT NULL,
    used    timestamptz,
    PRIMARY KEY (tenant, hash)
);

-- Every sign-in, the ones that fail too.
CREATE TABLE sign_in_log (
    tenant text NOT NULL DEFAULT current_setting('underdoc.tenant', true) REFERENCES tenants (id),
    id     bigint GENERATED ALWAYS AS IDENTITY,
    at     timestamptz NOT NULL DEFAULT now(),
    login  text NOT NULL CHECK (length(login) <= 200),
    method text NOT NULL,
    ok     boolean NOT NULL,
    reason text NOT NULL DEFAULT '' CHECK (length(reason) <= 200),
    PRIMARY KEY (tenant, id)
);

CREATE INDEX sign_in_log_recent ON sign_in_log (tenant, at DESC);

-- What a tenant enters for other systems, sealed by the vault (AES-GCM, key from the environment). Never shown again.
CREATE TABLE secrets (
    tenant  text NOT NULL DEFAULT current_setting('underdoc.tenant', true) REFERENCES tenants (id),
    name    text NOT NULL CHECK (name ~ '^[a-z0-9_.:-]{1,100}$'),
    sealed  bytea NOT NULL,
    changed timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant, name)
);

ALTER TABLE sign_in_settings ENABLE ROW LEVEL SECURITY;
ALTER TABLE sign_in_settings FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_only ON sign_in_settings
    USING (tenant = current_setting('underdoc.tenant', true)) WITH CHECK (tenant = current_setting('underdoc.tenant', true));

ALTER TABLE passkeys ENABLE ROW LEVEL SECURITY;
ALTER TABLE passkeys FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_only ON passkeys
    USING (tenant = current_setting('underdoc.tenant', true)) WITH CHECK (tenant = current_setting('underdoc.tenant', true));

ALTER TABLE sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE sessions FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_only ON sessions
    USING (tenant = current_setting('underdoc.tenant', true)) WITH CHECK (tenant = current_setting('underdoc.tenant', true));

ALTER TABLE ceremonies ENABLE ROW LEVEL SECURITY;
ALTER TABLE ceremonies FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_only ON ceremonies
    USING (tenant = current_setting('underdoc.tenant', true)) WITH CHECK (tenant = current_setting('underdoc.tenant', true));

ALTER TABLE invites ENABLE ROW LEVEL SECURITY;
ALTER TABLE invites FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_only ON invites
    USING (tenant = current_setting('underdoc.tenant', true)) WITH CHECK (tenant = current_setting('underdoc.tenant', true));

ALTER TABLE sign_in_log ENABLE ROW LEVEL SECURITY;
ALTER TABLE sign_in_log FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_only ON sign_in_log
    USING (tenant = current_setting('underdoc.tenant', true)) WITH CHECK (tenant = current_setting('underdoc.tenant', true));

ALTER TABLE secrets ENABLE ROW LEVEL SECURITY;
ALTER TABLE secrets FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_only ON secrets
    USING (tenant = current_setting('underdoc.tenant', true)) WITH CHECK (tenant = current_setting('underdoc.tenant', true));
