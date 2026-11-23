# Sign-in

Each tenant chooses how people get in ([ADR-23](../backlog/ADR-23-tenants-and-sign-in.md)); with one tenant, that is the
choice of the instance. An administrator of the tenant sets it on the page **Sign-in** of the administration
(`/admin/sign-in`, linked from the profile).

| Way | What it means |
|---|---|
| Passkey (default) | Local accounts sign in with a passkey of their device (WebAuthn). No login is typed: the passkey names the account. |
| Password and code | Local accounts sign in with a password and a code from an authenticator app (TOTP). Needs `UNDERDOC_SECRET_KEY`. |
| Password alone | Local accounts sign in with a password only. The weakest way. |
| Single sign-on (OIDC) | Any OIDC provider. The account is named by `preferred_username`, groups come from a claim. |
| Gate in front | A sign-in gate such as TinySesam passes the user on in headers (ADR-9). |

Several ways can be on at once. A session holds only as long as the way it came in is switched on: switching passwords
off signs out everyone who came in with one.

## Local accounts

- **Registration is off** by default. An administrator invites: the page makes a link that works once, for seven days,
  and is shown once. With registration on, `/register` lets anyone choose a login and go on to the same page; at most 20
  registrations an hour per tenant.
- The link leads to a page where the new account sets up a passkey, or a password (with the code of an app where the
  tenant asks for it). The account, its private notes and its first way in are made in one step; the link is used then.
- **Sign-in and security** (`/account/security`, linked from the profile) adds and removes passkeys, sets or changes the
  password (the current one is asked for), sets up or replaces the app for codes, and signs out everywhere else. The last
  way in cannot be removed.
- The first administrator of a tenant without any way in yet comes from the command line:

  ```bash
  underdoc invite --admin [--tenant <id>] anna "Anna Berg"   # prints the path /invite/<token>, valid for a week
  ```

## Security

- **Passwords** are stored as Argon2id hashes (19 MiB, two passes, one lane; OWASP), 12 to 200 characters. A login that
  does not exist takes as long as a wrong password, and both get the same answer.
- **The limit against guessing**: after five failed tries for a login of a tenant, each further try waits, twice as long
  each time, up to 15 minutes; a success clears it. It lives in the memory of the instance.
- **TOTP**: SHA-1, six digits, 30 seconds, one step early or late. A code works once. The secret is sealed with
  `UNDERDOC_SECRET_KEY`.
- **Passkeys**: the relying party is the host name of the tenant, so a passkey of one tenant is never offered on
  another. User verification and a discoverable credential are required. Each challenge is used once. A counter that
  does not go up marks a copied passkey, and that sign-in is refused.
- **OIDC**: the authorization code flow with PKCE (S256), a `state` bound to the browser, and a `nonce` in the ID
  token, which is checked for issuer, audience, expiry and signature. The provider must not let people choose their
  `preferred_username` freely. A local account's login is never taken by an account from outside.
- **Sessions** are a random token in the cookie `__Host-underdoc`: `HttpOnly`, `Secure`, `SameSite=Lax`, the path `/`,
  no domain. The database keeps only its SHA-256. A sign-in always starts a new session and ends the one the browser
  came with. A session ends after seven days without use, and after 30 days in any case.
- Every change is a POST that must come from a page of the same origin (`Sec-Fetch-Site` or `Origin`).
- **Every sign-in is logged**, failed ones with the reason, in the log of the tenant (shown on its sign-in page, kept 180
  days) and in the log of the instance. Neither ever holds a password, a code, a token or a secret.

## The gate

Whatever comes through the gate is checked by the gate's Go client ([ADR-32](../backlog/ADR-32-sign-in-only-through-the-gate.md),
TinySesam `clients/go`, pinned by commit). UnderDoc never reads who someone is, how they signed in or what kind of account
they have from a header on its own.

The headers of a gate can be forged by anyone who reaches UnderDoc without it. Only whoever runs the instance knows the
network, so `UNDERDOC_GATE` decides, not the administrator of a tenant:

| `UNDERDOC_GATE` | Meaning |
|---|---|
| empty (default) | The gate counts only where a tenant has it already. An instance that had accounts when sign-in arrived came through its gate, so its Main keeps the gate as its only way in, unchanged. A new instance starts with passkeys. |
| `on` | Administrators may switch the gate on for their tenant. |
| `off` | The headers never count, whatever a tenant says. |

How the gate reaches UnderDoc:

| Setting | Meaning | Default |
|---|---|---|
| `UNDERDOC_GATE_MODE` | `header`: the Remote headers the gate's proxy sets (forward auth, or the claims of a gate token the proxy checked). `token`: UnderDoc checks the gate token itself and ignores every Remote header | `header` |
| `UNDERDOC_GATE_PROXIES` | Header mode: the addresses or networks the proxy connects from, separated by commas. A request from any other address is refused | none: any address, and a warning at the start |
| `UNDERDOC_GATE_HEADER_USER`, `_NAME`, `_EMAIL`, `_GROUPS`, `_ID`, `_AUTH`, `_KIND` | Header mode: the names of the headers where the gate's `forward_headers` renames them | `Remote-User`, … |
| `UNDERDOC_GATE_ISSUER`, `UNDERDOC_GATE_AUDIENCE` | Token mode: the gate's base address and the audience the proxy names | none |
| `UNDERDOC_GATE_KEYS` or `UNDERDOC_GATE_JWKS_URL` | Token mode: the gate's public keys (`tinysesam gate-key`, comma separated), or the address of its key set | none |
| `UNDERDOC_GATE_COOKIE` or `UNDERDOC_GATE_TOKEN_HEADER` | Token mode: where the token comes from | the gate cookie |
| `UNDERDOC_GATE_KIND` | `required`: a request without `Remote-Kind` is refused, and a service account is one. Only once the gate says that its proxy sets `Remote-Kind` on every request | off |
| `UNDERDOC_ADMIN_ROLE` | The role of the app in the gate whose holders administer a tenant, unless the tenant names another group (`UNDERDOC_ADMIN_GROUP` is still read as its old name). The gate's own `admin` is never one | `underdoc-admin` |

**The duties of the proxy** in header mode, as the gate states them: it removes every Remote header the browser sent, on
every path, and sets them only from the gate's answer; UnderDoc is reachable through the proxy alone. Name the proxy's
addresses in `UNDERDOC_GATE_PROXIES`, so that headers from anywhere else count for nothing.

**Who someone is.** A person is bound by the lasting ID of their account in the gate (`Remote-Id`, the token's `ts_id`),
never by their login, name or mail. A request without it is not signed in. At the first request the account of
UnderDoc is made, keyed by that ID; a new login in the gate keeps the account. An account of the gate that UnderDoc keyed
by its ID before is bound to it by migration 031; an account keyed by its login waits for its binding (until then, its
person comes in as a new account). The roles in `Remote-Groups` become groups that can only be read; a request that did
not sign in as a session carries a part of the roles only, so only a session changes the groups of an account.

**How someone signed in.** The client reads `Remote-Auth` (`session`, `oauth`, `apikey`, `app-password`). Every request
that did not sign in as a session belongs to the built-in group Agents, a missing or unknown value included
([rights.md](rights.md#the-group-agents)): it may never manage rights, share, delete for good or read passwords, and an
administrator's agent administers nothing. A gate token from before TinySesam 0.26.3 has no `ts_auth`, so it is an agent
until it expires. A session of the sign-in pages (passkey, password, single sign-on) is always a session.

**What kind of account.** `Remote-Kind` says person (`user`) or service account (`service`). A proxy older than the
gate's spec version 2 passes on what the browser sent there, so it is not yet the only barrier: by default it is read and
kept with the account, a request without it comes in as of unknown kind, and a service account counts as a person signed
in with a token, an agent. `UNDERDOC_GATE_KIND=required` makes it decide. In token mode the kind is signed, so it always
decides.

Where the gate is the only way in, a request without it gets the page "Not signed in", and none of the pages of signing
in answers. `UNDERDOC_DEV_USER` still stands in for the gate on one machine, as the account with the lasting ID 1 and the
role of the app.

## Secrets of other systems

What a tenant enters for other systems (the client secret of OIDC today; sources, mailboxes and file sources later) is
sealed with AES-256-GCM under `UNDERDOC_SECRET_KEY` (32 random bytes in base64, for example `openssl rand -base64 32`),
bound to its tenant and its name, so a sealed value copied elsewhere does not open. It can be written and used, never
shown again, and never reaches a log. Without the key, the ways that need it are not offered. Package `vault` is the
building block for later modules: `vault.Store{Vault, Rows: db}` with `Put`, `Has`, `Use` and `Delete`. The same vault
seals the passwords in the text and the NT hashes of Samba. **Changing the key** (2026-11-20): start with the new key in
`UNDERDOC_SECRET_KEY` and the old one in `UNDERDOC_SECRET_KEY_OLD`, run `underdoc rekey`, then drop the old key
([secrets.md](secrets.md#the-key)); before, it meant entering the secrets and setting up the apps for codes again.

## Libraries

| Library | Version | Licence | For |
|---|---|---|---|
| [go-webauthn/webauthn](https://github.com/go-webauthn/webauthn) | 0.18.2 | BSD-3-Clause | passkeys |
| [pquerna/otp](https://github.com/pquerna/otp) | 1.5.0 | Apache-2.0 | TOTP and its picture |
| [coreos/go-oidc](https://github.com/coreos/go-oidc) | 3.21.0 | Apache-2.0 | OIDC discovery and ID tokens |
| [golang.org/x/oauth2](https://github.com/golang/oauth2) | 0.38.0 | BSD-3-Clause | the code flow with PKCE |
| [golang.org/x/crypto](https://github.com/golang/crypto) | 0.59.0 | BSD-3-Clause | Argon2id |
| [TinySesam gate client](https://github.com/Ollornog/TinySesam/tree/main/clients/go) | commit `4bb607f` (0.29.0) | MIT | the gate: headers and gate token |

Through them come go-webauthn/x (BSD-3-Clause), go-jose (Apache-2.0), golang-jwt (MIT), fxamacker/cbor (MIT), google/go-tpm (Apache-2.0),
google/uuid (BSD-3-Clause), go-viper/mapstructure (MIT), tinylib/msgp (MIT), philhofer/fwd (MIT), x448/float16 (MIT)
and boombuler/barcode (MIT).
