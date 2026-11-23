---
type: Decision
title: Sign-in only through the gate
status: erledigt
id: ADR-32
tags: [auth, sign-in, tenants]
created: '2026-11-23'
---
# ADR-32 — Sign-in only through the gate

## Context

ADR-23 let each tenant choose its way in: local accounts with a passkey, with a password and TOTP or with a password
alone, OIDC, or the gate of ADR-9. Every one of these ways had to be built, hardened and attacked in UnderDoc, and each
tenant had to keep accounts of its own next to those of the gate.

Since then the gate, TinySesam, does all of it: passkeys, passwords with TOTP, OIDC, LDAP, app passwords, API keys and
OAuth for programs, the account lifecycle and approvals (ADR-35). It has a Go client that checks what reaches an app: the
Remote headers its proxy sets, or its signed gate token. ADR-35 set the line: the gate decides who someone is, whether
they may use UnderDoc, its coarse roles, how they signed in and what kind of account they have; UnderDoc decides
everything about its objects. ADR-36 needs the way someone signed in to be trustworthy, because the group Agents rests on
it. The maintainer decided on 2026-11-22 that UnderDoc signs in only through the gate.

The state of the gate on 2026-11-23 (version 0.29.0):

- The Go client (`clients/go`) has no release tag yet; it is used by commit.
- It reads the headers (header mode) or checks the gate token (token mode). Token mode cannot be used with the gate's
  proxy templates yet: they remove the gate cookie before the app (the gate's T-84).
- The client follows its spec version 2: it refuses a request without `Remote-Kind`. A proxy older than that spec passes
  a `Remote-Kind` the browser sent through, and the proxies in front of UnderDoc are not all updated yet. The gate will
  say when `Remote-Kind` is the only barrier it needs to be.
- Gate tokens issued before 0.26.3 carry no `ts_auth`.
- Tenants are not settled in the gate (its T-73).

## Options

- **Keep ADR-23:** every way in stays in UnderDoc, with its code, its tables and its attack surface, twice next to the
  gate.
- **The gate only, read by hand:** the headers read as before, without the client. Every rule of the client (one value
  per header, no control characters, a numeric ID, the kind against the way of signing in, the trusted proxies, the
  token's signature) would have to be written again.
- **The gate only, through its Go client:** UnderDoc keeps no credentials of a person and reads nothing about who
  someone is from a header on its own.

## Decision

The third option, decided by the maintainer on 2026-11-22. It replaces the part "Sign-in is chosen per tenant" of ADR-23
and the registration and passwords of local accounts with it; the rest of ADR-23 (tenants in the data model, row-level
security, the hoster) stays. ADR-9 is right again in what it decided: the gate in front.

- **The gate's Go client checks every request**, pinned by commit (TinySesam 0.29.0, `4bb607f`). Header mode is the
  default; token mode is configured and tested for when the gate delivers its token to the app.
- **A person is bound by `Remote-Id`** (the token's `ts_id`), never by a login, a name or a mail address. A request
  without it is refused.
- **Groups** are the roles of the gate in `Remote-Groups`, taken over as groups that can only be read (ADR-29, ADR-35,
  ADR-36).
- **Administrators** hold the role of the app in the gate, `underdoc-admin` unless configured otherwise. The gate's own
  administrator is never a right in UnderDoc.
- **`Remote-Auth` and `Remote-Kind` come only from the checked result** of the client. Every sign-in that is not
  `session` belongs to Agents (ADR-36), a missing or unknown value included; a token without `ts_auth` too.
- **`Remote-Kind` is not yet the only barrier.** It is read and kept with the account, but a service account counts as
  one only once a switch says that the gate's proxy sets it on every request. Until then a request without it is let
  through as of unknown kind, and a service account is an agent. With the switch on, a request without it is refused.
- **No sign-in of UnderDoc's own:** passkeys, passwords, TOTP, OIDC, registration and invitations go. What a person
  set up in the gate, they change in the gate.
- **An emergency access on the command line** only: whoever runs the instance can mark an account as an administrator,
  for a gate that lost its role.
- **What stays:** the app passwords of Samba (ADR-10, ADR-30), since the share cannot use the gate, and share links with
  their own token (ADR-29).
- **One tenant.** The tenant fields stay a placeholder until the gate decides how it knows tenants (its T-73). The names
  of the headers are configurable as the gate's `forward_headers` names them.

## Consequences

- The tables of the sign-in of ADR-23 go. Accounts made before keep their content: an account of the gate keyed by its
  ID is bound to it, every other one is bound by whoever runs the instance on the command line.
- Whoever runs UnderDoc must keep the duties of the gate's proxy: it removes every Remote header the browser sent, and
  UnderDoc is reachable through it alone. Naming the proxy's addresses lets UnderDoc refuse headers from anywhere else.
- Sessions, sign-out and the security of an account are the gate's. UnderDoc links to it.
- When the gate says that `Remote-Kind` is the only barrier, the switch goes on, and service accounts get what is granted
  to them (ADR-35).
- When the gate decides T-73, the tenant comes from the gate, and many tenants become possible again.
- **Built on 2026-11-23, part 1** ([docs/sign-in.md](../docs/sign-in.md#the-gate), migration 031, T-27). Settled in the
  build:
  - The client sits in package `account` (`account.Gate`), as that package already knew who is signed in; no package of
    its own.
  - The client refuses a request without `Remote-Kind`. While the switch is off, UnderDoc hands it such a request with
    `Remote-Kind: user` filled in, so that the client checks everything else, and keeps the kind as unknown. Filled in or
    sent, the kind can only make a request weaker: without the switch, `service` makes no service account. Only the
    presence of the header is read from the request itself, never its value. In token mode the kind is signed, so it
    always decides.
  - The account keeps its key, and the ID of the gate has a column of its own (`remote_id`), since every table refers to
    the key. A new account is keyed by the ID of the gate, as before; where that key is taken by an account under a login,
    the new one gets a key of its own rather than taking that account.
  - A request that is no session does not change the groups of its account: the gate's tokens carry a part of the roles,
    and the client says never to take a role away because a token lacks it.
  - `UNDERDOC_GATE_PROXIES` is not required: a proxy in front of a container connects from an address that only the
    installation knows. Without it, UnderDoc warns at the start.
  - `UNDERDOC_ADMIN_GROUP` is still read as the old name of `UNDERDOC_ADMIN_ROLE`, so an instance keeps its
    administrators until it is renamed.
- **Built on 2026-11-23, part 2** ([docs/sign-in.md](../docs/sign-in.md), migration 032, T-27). Settled in the build:
  - Migration 032 drops the ways in chosen per tenant, passkeys, sessions, ceremonies, invitations, the log of sign-ins,
    the passwords and TOTP secrets of local accounts, and the sealed client secret of OIDC. The libraries for passkeys,
    TOTP and OIDC leave UnderDoc's own code; the time tracking (EightPerDay) still brings two of them along.
  - The emergency access is a mark on the account, the column `admin` that local administrators had: `underdoc admin
    grant|revoke <remote-id>` sets it by the lasting ID, and makes the account bound to that ID where there is none yet,
    so it works before the first sign-in. The web only takes a mark back, never gives one, and nobody takes their own.
    The lock that kept two administrators from taking each other's administration at once (ADR-29) now guards the marks:
    an administrator without the role must still be marked.
  - Accounts made before are bound with `underdoc account bind <key|login> <remote-id>`, never by name on their own;
    an ID bound elsewhere and an account bound to another ID are refused. A local administrator keeps the mark.
  - The address of a share link is open at the gate, so nobody is signed in there. A link to comment or edit now leads
    to `/link/<token>`, behind the gate, where the person takes its role; the password of a link holds there as well.
  - Many tenants do not start (`UNDERDOC_TENANTS=many`): without a tenant from the gate, every person of the gate would
    come into every tenant. The code of many tenants and the hoster stays, tested, without invitations: the
    administrators of a tenant and of the hoster are those the gate gives the role of the app on its host.
  - `UNDERDOC_GATE=off` stops the start, since nobody could sign in; any other value of it only warns.
  - A request without the gate gets the page "Not signed in" for the API too, as an instance behind the gate did before.
- **Built on 2026-11-23, part 3** ([docs/sign-in.md](../docs/sign-in.md#the-gate), migration 035, B-3). The gate's
  proxies set `Remote-Kind` on every request and remove what the browser sent (TinySesam, 2026-11-23), so the switch
  went on and became the default. Settled in the build:
  - `Remote-Kind` is required in header mode as in token mode: a request without it, empty or with a value other than
    `user` or `service` is refused with 403 (the page "Refused"), as the gate's client refuses it. The filling in of a
    missing kind as `user` is gone from the default. `UNDERDOC_GATE_KIND=legacy` is the one explicit way back, header
    mode only, for a proxy that does not set it yet, with a warning at the start; an unknown value stays refused there
    too. Empty or `required` is the default; any other value stops the start. Proven by `TestGateKind` and
    `TestServiceAccountsThroughTheAPI`.
  - A request the gate passed on but its client refuses for a malformed Remote header or claim (`header`, `claims`) is
    answered 403, not 401: someone was signed in, but not in a way UnderDoc takes. A request without the gate stays 401.
  - Service accounts no longer hang on the switch: one comes in only with a lasting ID in `UNDERDOC_SERVICE_ACCOUNTS`,
    and an allowed ID is a service account whatever kind a request names, so a proxy that passed on a wrong kind cannot
    turn it into a person. Refusing a service account nobody allowed only makes a request weaker, so it holds in the way
    back as well, as far as the proxy sends the kind: without it, the way back takes the request as a person's, which
    the warning at the start says (B-4). The list is a setting rather than a table: whoever runs the instance (the fleet's inventory) sets it
    once, idempotently, and it needs no command of its own; the start marks the accounts of those IDs
    (`accounts.service`), so every decision about them, also on the share, knows them.
  - The role to create spaces is read like the administrator's: `UNDERDOC_SPACES_ROLE`, `underdoc-spaces` unless
    configured, compared exactly; the gate's own `admin` stops the start.
  - The gate's Go client has a release tag now: `go.mod` pins `clients/go v0.29.0` instead of the pseudo-version of
    commit `4bb607f`, the same commit, so nothing changes in what checks a request.
