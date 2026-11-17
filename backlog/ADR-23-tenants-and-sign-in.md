---
id: ADR-23
type: Decision
title: One or many tenants, and sign-in built in next to the gate
status: erledigt
tags: [architecture, auth, tenants]
created: 2026-11-16
---

## Context

UnderDoc is meant for a single person, for small companies and for hosters that offer it to many customers (the maintainer,
2026-11-16). ADR-5 allowed only one instance per customer, and ADR-9 only the sign-in gate in front. A single user or a hoster
usually has neither a gate nor an identity provider, and a hoster wants many customers in one instance.

## Options

- **Keep ADR-5 and ADR-9:** simple, but useless for single users and hosters.
- **Tenants later:** start with one instance per customer and add tenants afterwards. Every table and query would have to be
  touched again once the many new tables of the following milestones exist.
- **Tenants in the data model now, with one tenant as the default.**

## Decision

The third option, settled with the maintainer on 2026-11-16. This replaces ADR-5 and ADR-9.

- **Two modes, chosen at installation:** *one tenant* (the default, nothing about tenants shows) and *many tenants*.
- **Every table carries its tenant**, and PostgreSQL row-level security enforces the separation: the app sets the tenant
  of a request once, and a query without it sees nothing. A test proves that no tenant can read, find, embed or export
  anything of another, through every door: pages, search, files, embeddings, the API and exports.
- **A tenant is found by its host name:** a subdomain of the hoster by default, and optionally a domain of its own. TLS is the
  job of the proxy in front. Paths are not used, as they would share cookies and the CSP between tenants.
- **A hoster administrator manages tenants only:** create, lock, delete, set quotas (storage, users) and allow modules. That
  role sees no content. Each tenant has administrators of its own.
- **Sign-in is chosen per tenant** (with one tenant: per instance), any of:
  - local accounts with a **passkey** (the default),
  - local accounts with name, password and **TOTP**,
  - local accounts with name and password only,
  - **OIDC** with any provider (PocketID, Keycloak, Entra, Google), groups from a claim, the account named by
    `preferred_username`,
  - the **gate** of ADR-9: trusted, signed headers of a gate such as TinySesam.
- **Registration is off by default** and can be switched on per tenant; otherwise administrators invite.
- Passwords are stored with Argon2id. Sign-in has a limit against guessing, sessions are cookies with `HttpOnly`, `Secure`
  and `SameSite`, and every sign-in is logged.

## Consequences

- ADR-5 and ADR-9 are replaced. A gate in front stays a valid way, and this instance keeps it (ADR-9 still describes it).
- Credentials of other systems that a tenant enters (sources, mail, file sources) are stored encrypted with AES-GCM under a
  key from the environment, can only be written, never shown again, and never reach a log (2026-11-16).
- Search indexes, the content store and caches are separated per tenant as well.
- Starting with one instance per customer stays possible: the mode *one tenant* is exactly that.
- Built on 2026-11-17 ([docs/tenants.md](../docs/tenants.md), [docs/sign-in.md](../docs/sign-in.md)). Settled in the
  build: whether the headers of a gate may count is decided by whoever runs the instance (`UNDERDOC_GATE`), never by the
  administrator of a tenant, since anyone who reaches UnderDoc without the gate could forge them; an instance that had
  accounts before keeps its gate. The folder source belongs to the tenant Main.
