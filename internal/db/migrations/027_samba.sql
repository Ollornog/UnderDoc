-- SPDX-License-Identifier: MIT
-- Samba (ADR-30): the libraries shared over SMB, the Samba users of a tenant, and what the working copy on disk last
-- agreed on with the database.

-- A collection of the view Files is shared over Samba when its manager switches it on; the folder of the tenant and the
-- folders of users are shared by the setup and by the app passwords, not by this flag.
ALTER TABLE file_roots ADD COLUMN samba boolean NOT NULL DEFAULT false;

-- An account with an app password for SMB: its Samba user name, the UID it has in the companion, and the NT hash of the
-- app password sealed by the vault (bound to the tenant and to samba:<account>); the password itself is never kept.
CREATE TABLE samba_accounts (
    tenant  text NOT NULL DEFAULT current_setting('underdoc.tenant', true) REFERENCES tenants (id),
    account text NOT NULL,
    name    text NOT NULL CHECK (name ~ '^[a-z][a-z0-9._-]{0,31}$'),
    uid     integer NOT NULL CHECK (uid BETWEEN 1000 AND 65534),
    nt_hash bytea NOT NULL,
    created timestamptz NOT NULL DEFAULT now(),
    changed timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant, account),
    UNIQUE (tenant, name),
    UNIQUE (tenant, uid),
    FOREIGN KEY (tenant, account) REFERENCES accounts (tenant, id) ON DELETE CASCADE
);

-- What the working copy and the database last agreed on, per object of a shared library: where it lies, and the inode,
-- size, time and hash of the file there. A row outlives its object on purpose: an object purged from the database still
-- has its row, so its file is taken off the disk instead of coming back as a new one.
CREATE TABLE share_files (
    tenant   text NOT NULL DEFAULT current_setting('underdoc.tenant', true) REFERENCES tenants (id),
    object   text NOT NULL,
    root     text NOT NULL,
    path     text NOT NULL,
    kind     text NOT NULL CHECK (kind IN ('folder', 'file')),
    inode    bigint NOT NULL DEFAULT 0,
    size     bigint NOT NULL DEFAULT 0,
    mtime_ns bigint NOT NULL DEFAULT 0,
    sha256   text NOT NULL DEFAULT '',
    PRIMARY KEY (tenant, object)
);

CREATE INDEX share_files_by_root ON share_files (tenant, root);

DO $$
DECLARE t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['samba_accounts', 'share_files'] LOOP
        EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
        EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', t);
        EXECUTE format('CREATE POLICY tenant_only ON %I USING (tenant = current_setting(''underdoc.tenant'', true))
            WITH CHECK (tenant = current_setting(''underdoc.tenant'', true))', t);
    END LOOP;
END $$;

-- Every change of the files, of their versions, of rights, groups and Samba users tells the working copy of the tenant to
-- look (channel underdoc_share, the tenant as payload). Whatever door made the change, the web, an import or a link, the
-- disk follows; nothing has to remember to call it.
CREATE FUNCTION share_notify() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        PERFORM pg_notify('underdoc_share', OLD.tenant);
    ELSE
        PERFORM pg_notify('underdoc_share', NEW.tenant);
    END IF;
    RETURN NULL;
END
$$;

-- Of the objects, only those of the view Files.
CREATE FUNCTION share_notify_object() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        IF OLD.area = 'files' THEN
            PERFORM pg_notify('underdoc_share', OLD.tenant);
        END IF;
    ELSIF NEW.area = 'files' THEN
        PERFORM pg_notify('underdoc_share', NEW.tenant);
    END IF;
    RETURN NULL;
END
$$;

CREATE TRIGGER share_notify AFTER INSERT OR UPDATE OR DELETE ON objects FOR EACH ROW EXECUTE FUNCTION share_notify_object();
CREATE TRIGGER share_notify AFTER INSERT OR DELETE ON file_versions FOR EACH ROW EXECUTE FUNCTION share_notify();
CREATE TRIGGER share_notify AFTER INSERT OR UPDATE OR DELETE ON file_roots FOR EACH ROW EXECUTE FUNCTION share_notify();
CREATE TRIGGER share_notify AFTER INSERT OR UPDATE OR DELETE ON grants FOR EACH ROW EXECUTE FUNCTION share_notify();
CREATE TRIGGER share_notify AFTER INSERT OR UPDATE OR DELETE ON group_members FOR EACH ROW EXECUTE FUNCTION share_notify();
CREATE TRIGGER share_notify AFTER INSERT OR UPDATE OR DELETE ON samba_accounts FOR EACH ROW EXECUTE FUNCTION share_notify();
CREATE TRIGGER share_notify AFTER INSERT OR UPDATE OR DELETE ON share_link_users FOR EACH ROW EXECUTE FUNCTION share_notify();
