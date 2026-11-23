# Sign-in

UnderDoc signs in only through the gate in front of it, TinySesam
([ADR-32](../backlog/ADR-32-sign-in-only-through-the-gate.md)). The gate keeps the ways in (passkeys, passwords with
TOTP, OIDC, LDAP, app passwords, API keys, OAuth for programs), the sessions, sign-out, the account lifecycle and
approvals; UnderDoc keeps none of them and has no sign-in page of its own. What a person set up, they change on the
gate's account page. The profile signs out at the gate (`/.tinysesam/logout`).

Before ADR-32, each tenant chose its way in ([ADR-23](../backlog/ADR-23-tenants-and-sign-in.md)): local accounts with a
passkey or a password, OIDC, or the gate. Migration 032 removed those tables; how accounts from then come in is under
[Accounts made before](#accounts-made-before).

## The gate

Whatever comes through the gate is checked by the gate's Go client (TinySesam `clients/go`, pinned by its module tag `clients/go/v0.29.0`). UnderDoc
never reads who someone is, how they signed in or what kind of account they have from a header on its own.

| Setting | Meaning | Default |
|---|---|---|
| `UNDERDOC_GATE_MODE` | `header`: the Remote headers the gate's proxy sets (forward auth, or the claims of a gate token the proxy checked). `token`: UnderDoc checks the gate token itself and ignores every Remote header | `header` |
| `UNDERDOC_GATE_PROXIES` | Header mode: the addresses or networks the proxy connects from, separated by commas. A request from any other address is refused | none: any address, and a warning at the start |
| `UNDERDOC_GATE_HEADER_USER`, `_NAME`, `_EMAIL`, `_GROUPS`, `_ID`, `_AUTH`, `_KIND` | Header mode: the names of the headers where the gate's `forward_headers` renames them | `Remote-User`, … |
| `UNDERDOC_GATE_ISSUER`, `UNDERDOC_GATE_AUDIENCE` | Token mode: the gate's base address and the audience the proxy names | none |
| `UNDERDOC_GATE_KEYS` or `UNDERDOC_GATE_JWKS_URL` | Token mode: the gate's public keys (`tinysesam gate-key`, comma separated), or the address of its key set | none |
| `UNDERDOC_GATE_COOKIE` or `UNDERDOC_GATE_TOKEN_HEADER` | Token mode: where the token comes from | the gate cookie |
| `UNDERDOC_GATE_KIND` | Empty or `required`: a request without `Remote-Kind`, or with a value other than `user` or `service`, is refused (403). `legacy`: the way back for a proxy that does not set `Remote-Kind` yet, header mode only, with a warning at the start | `required` |
| `UNDERDOC_ADMIN_ROLE` | The role of the app in the gate whose holders administer UnderDoc (`UNDERDOC_ADMIN_GROUP` is still read as its old name). The gate's own `admin` is never one, and naming it stops the start | `underdoc-admin` |
| `UNDERDOC_SPACES_ROLE` | The role of the app in the gate whose holders may create spaces, the collections at the top ([rights.md](rights.md#spaces)). Administrators may anyway | `underdoc-spaces` |
| `UNDERDOC_SERVICE_ACCOUNTS` | The lasting IDs (`Remote-Id`) of the service accounts allowed in, separated by commas. Any other service account is refused (403) | none |

The names of the claims of a gate token (`ts_user`, `ts_id`, `ts_auth`, `ts_kind`, …) are fixed by the gate's
specification. `UNDERDOC_GATE` is gone: an instance with `UNDERDOC_GATE=off` does not start, since nobody could sign in.

**The duties of the proxy** in header mode, as the gate states them: it removes every Remote header the browser sent, on
every path, and sets them only from the gate's answer; UnderDoc is reachable through the proxy alone. Name the proxy's
addresses in `UNDERDOC_GATE_PROXIES`, so that headers from anywhere else count for nothing. Only the addresses of share
links (`/s/…`) are open at the gate; everything else, `/link/…` included, needs a sign-in there.

**Who someone is.** A person is bound by the lasting ID of their account in the gate (`Remote-Id`, the token's `ts_id`),
never by their login, name or mail. A request without it is not signed in. At the first request the account of
UnderDoc is made, keyed by that ID; a new login in the gate keeps the account. The roles in `Remote-Groups` become groups
that can only be read; a request that did not sign in as a session carries a part of the roles only, so only a session
changes the groups of an account.

**Administrators** hold the role of the app (`UNDERDOC_ADMIN_ROLE`), compared exactly. An account can also be marked as
an administrator on the command line, the emergency access below. Administrators in the web take such a mark back; they
give it to nobody.

**How someone signed in.** The client reads `Remote-Auth` (`session`, `oauth`, `apikey`, `app-password`). Every request
that did not sign in as a session belongs to the built-in group Agents, a missing or unknown value included
([rights.md](rights.md#the-group-agents)): it may never manage rights, share, delete for good or read passwords, and an
administrator's agent administers nothing. A gate token from before TinySesam 0.26.3 has no `ts_auth`, so it is an agent
until it expires.

**What kind of account.** `Remote-Kind` says person (`user`) or service account (`service`). The gate's proxies set it on
every request and remove what the browser sent (TinySesam, 2026-11-23), so it is required: a request without it, empty,
or with any other value is refused (403), as the gate's client refuses it. `UNDERDOC_GATE_KIND=legacy` is the way back
for a proxy that does not set it yet: a missing `Remote-Kind` is then filled in as `user`, so that the client checks
everything else, and kept as unknown; an unknown value stays refused. In token mode the kind is signed and always
required.

**Service accounts** come in only with a lasting ID an administrator allowed in `UNDERDOC_SERVICE_ACCOUNTS`; any other
request the gate names a service account is refused (403, the page "Refused"). An allowed ID is a service account
whatever kind a request names, and a service account is always an agent
([rights.md](rights.md#service-accounts)): it never belongs to everyone signed in or to a role of the gate, sees the
folder source empty, and gets only what is granted to it or to a local group it is put in. The list is read at the start,
which marks the accounts of those IDs in the database, so it holds for every decision about them.

A request without the gate gets the page "Not signed in" (401), the API included. A request the gate passed on but the
client refuses (a malformed Remote header or claim, a missing or unknown kind) and a service account nobody allowed get
the page "Refused" (403). `UNDERDOC_DEV_USER` stands in for the
gate on one machine, as the account with the lasting ID 1 and the role of the app.

## Share links

A link carries a token of its own and needs no sign-in ([rights.md](rights.md)). A link to comment or edit is taken by
an account of the tenant: its page leads to `/link/<token>`, behind the gate, where the signed-in person holds the role
from then on; an agent never takes one. The password of a link holds for both addresses.

## The emergency access

For a gate that lost the role of the app, whoever runs the instance marks an account as an administrator on the command
line, by the lasting ID of the gate, also before its first sign-in:

```bash
underdoc admin [--tenant <id>] grant <remote-id>    # marks the account of that lasting ID; made if need be
underdoc admin [--tenant <id>] revoke <remote-id>   # takes the mark back
underdoc account [--tenant <id>] list               # every account, its lasting ID, its source and its mark
```

The mark holds only for a session of that ID: someone with the same login and another ID is no administrator, and its
agent administers nothing. Take the mark back once the gate gives the role again.

## Accounts made before

Migration 031 binds every account of the gate that UnderDoc keyed by its ID to that ID. Every other account (one of the
gate keyed by its login because the gate sent no ID, a local account, an account of OIDC) keeps all it owns, but nobody
comes into it until whoever runs the instance binds it to the lasting ID of its person in the gate:

```bash
underdoc account bind [--tenant <id>] <key|login> <remote-id>
```

An ID bound to another account, or an account bound to another ID, is refused, so nobody takes over someone else's
account. Until it is bound, its person comes in as a new account. A local administrator stays marked, and administers
again once bound.

## Tenants

UnderDoc runs one tenant (ADR-32): the gate does not name a tenant yet (its T-73), so every person of the gate would
come into every tenant. An instance with `UNDERDOC_TENANTS=many` does not start; the tenants stay in the data model
([tenants.md](tenants.md)).

## Secrets of other systems

What a tenant enters for other systems (sources, mailboxes and file sources later) is sealed with AES-256-GCM under
`UNDERDOC_SECRET_KEY` (32 random bytes in base64, for example `openssl rand -base64 32`), bound to its tenant and its
name, so a sealed value copied elsewhere does not open. It can be written and used, never shown again, and never reaches
a log. Without the key, the ways that need it are not offered. Package `vault` is the building block for later modules:
`vault.Store{Vault, Rows: db}` with `Put`, `Has`, `Use` and `Delete`. The same vault seals the passwords in the text and
the NT hashes of Samba. **Changing the key** (2026-11-20): start with the new key in `UNDERDOC_SECRET_KEY` and the old
one in `UNDERDOC_SECRET_KEY_OLD`, run `underdoc rekey`, then drop the old key ([secrets.md](secrets.md#the-key)).

## Libraries

| Library | Version | Licence | For |
|---|---|---|---|
| [TinySesam gate client](https://github.com/Ollornog/TinySesam/tree/main/clients/go) | `v0.29.0` (commit `4bb607f`) | MIT | the gate: headers and gate token, standard library only |
| [golang.org/x/crypto](https://github.com/golang/crypto) | 0.59.0 | BSD-3-Clause | Argon2id of the passwords of share links |
