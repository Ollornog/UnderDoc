# Tenants

UnderDoc runs for one person, for a small company, or for a hoster that offers it to many customers in one instance
([ADR-23](../backlog/ADR-23-tenants-and-sign-in.md)). Every table carries its tenant, and PostgreSQL keeps the tenants apart by row-level security.

## Two modes

| `UNDERDOC_TENANTS` | What it means |
|---|---|
| `one` (default) | One tenant, called Main. Nothing about tenants shows, every host name leads to it. |
| `many` | Each tenant is found by the host name of the request. Needs the database. |

With many tenants, `acme.<UNDERDOC_TENANT_DOMAIN>` leads to the tenant `acme`, and a tenant may have domains of its own
next to that. TLS for all of them is the job of the proxy in front, which passes the `Host` header on unchanged. Paths
are never used to tell tenants apart, because tenants on one host would share cookies and the content security policy.

- An unknown host name, the bare domain and an archived tenant answer "Nothing here" (404).
- A locked tenant answers "Locked" (403) and keeps everything.
- The tenant `hoster` is reserved for the hoster administrators and is never a workspace.
- The folder source (`UNDERDOC_DIR`) belongs to the instance, so only Main shows it.

```bash
underdoc tenant add acme "Acme Ltd"                       # a new, empty tenant at acme.<domain>
underdoc export --tenant acme ./acme-backup                # everything of one tenant
underdoc import --tenant acme --owner anna ./dir "Team"    # into one tenant; without --tenant, Main
```

## The hoster administration

With many tenants, `UNDERDOC_HOSTER_HOST` names a host of its own for the hoster. Its administrators are accounts of the
reserved tenant `hoster`; the first one comes from `underdoc invite --tenant hoster --admin <login> <name>`, and they
sign in as `/admin/sign-in` of that host sets (a passkey by default). On that host `/hoster` lists the tenants with what
they use, and nothing of a workspace answers there.

- **Make a tenant**: its ID (the subdomain), its name, and the login and name of its first administrator, who gets a
  link that works once, for seven days. A new tenant signs in with passkeys and has no registration.
- **Rename** gives it another name; its ID, and with it its subdomain, stays.
- **Lock** lets nobody of the tenant in; nothing is lost. **Archive** is the delete of the hoster: the tenant is gone
  from every host, its data stays, and **Restore** brings it back whole.
- **Delete for good**, only for an archived tenant and only with its ID typed: every row of it in every table, its
  addresses, its files in the content store and its search index go, in one transaction that works as that tenant, so
  row-level security keeps every other one out of reach. Every table refers to `tenants`, so a row left behind would stop
  the delete instead of staying. Its ID is free again afterwards. Main and the hoster tenant cannot be deleted.
- **Limits**: users at most and storage at most (the bytes of the current blocks of its pages, plus its files with all
  their versions, attachments and running uploads; versions of blocks are not counted). Past the limit, no new account comes in, from the gate, OIDC or an invitation, while the accounts there keep
  signing in; and no block, copy, import, template or upload grows the storage, while shrinking always works. The quotas of the files
(docs/files.md) hold next to it.
- **Modules** it may use, of those the instance allows ([docs/modules.md](modules.md)).
- **Domains of its own**, next to the subdomain, and more administrators by invitation.
- The hoster sees names, states and counts, never content. Its session is one of the tenant `hoster`, which row-level
  security keeps from every other, and its cookie belongs to its host.

## How the separation works

- Every table but `tenants`, `tenant_hosts` and `schema_migrations` has a column `tenant`. Its default is the setting
  `underdoc.tenant`, so an insert cannot name another tenant, and a policy for reading and writing allows only rows of
  that setting: `tenant = current_setting('underdoc.tenant', true)`. Row-level security is on and forced, so it also
  holds for the owner of the tables.
- Each connection the application takes from its pool is set to the tenant of the request it serves, and to no tenant
  outside a request. A query without a tenant sees nothing and writes nothing.
- Keys and references carry the tenant: an object ID is unique within its tenant, an import never collides with another
  tenant, and no row can point at a row of another tenant.
- Jobs such as emptying the trash run once per tenant, each as that tenant.

**The database role.** UnderDoc migrates the schema as the role of `UNDERDOC_DATABASE_URL`. If that role is a
superuser, row-level security does not hold for it, so the application creates the role `underdoc_app` (no login, no
`BYPASSRLS`), grants it the tables, and every connection of the pool works as that role. A role with `BYPASSRLS` that
is not a superuser cannot step down, and UnderDoc refuses to start with it. The best setup is a role that owns the
database and is neither: then `FORCE ROW LEVEL SECURITY` holds the rules for it directly, and nothing in a session can
lift them. A superuser session could leave `underdoc_app` again by `RESET ROLE`; the application never sends that, and
all its queries take their values as parameters.

**Migrations that change rows** run as the owner of the tables, under `FORCE ROW LEVEL SECURITY`. Such a migration turns
`FORCE` off for its table and on again within its own transaction.

## The guard

`TestEveryTableKeepsTenantsApart` in `internal/db` reads the schema and fails for every table, also one added later,
that lacks any of:

- a column `tenant` that may not be empty and fills itself from the setting,
- row-level security, turned on and forced,
- permissive policies that all keep to the tenant for every command, for reading and writing,
- a primary key that starts with the tenant,
- the tenant on both sides of every reference to another table of tenants.

`TestNoTableShowsAnotherTenant` fills a tenant and reads every table as another tenant and as none. The separation test
through every door of the web, `TestTenantsSeeNothingOfEachOther` in `internal/web`, works in two tenants as the same
person and checks the tree, pages, search, embeddings, links, comments, uses, versions, the trash, the export and every
change: nothing of one tenant reaches the other.

A new table copies the pattern of `internal/db/migrations/013_tenants.sql`: the column, the default, the two `ALTER
TABLE … ROW LEVEL SECURITY` and the policy `tenant_only`.

Sequences are shared: a comment number or a version number tells nothing but that some tenant made something.
