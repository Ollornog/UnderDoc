---
type: Task
title: 'TinySesam built in: its own gate in UnderDoc''s compose, local accounts and OIDC mixed'
status: offen
sprint: später
points: 5
id: T-39
milestone: M-32
created: '2026-11-23'
---
# T-39 — TinySesam built in: its own gate in UnderDoc's compose, local accounts and OIDC mixed

Decided by the maintainer on 2026-11-23: TinySesam is always built into UnderDoc, as a gate right in front of the app, not only an outside gate. Local accounts and OIDC providers (PocketID, Entra and others) run mixed in one instance. Like Seafile, Immich or Paperless an install works on its own with its own accounts, but the sign-in stays in one hardened place (ADR-32, ADR-35).

- UnderDoc's compose file brings a TinySesam server as a sidecar (TinySesam server mode, its M-4). Overhead: one process of about 70 MiB, idle CPU close to zero.
- Configuration through TinySesam's TOML file with apps.underdoc: account API (provisioning, allowlist of roles), approval types (underdoc.object.purge, underdoc.trash.empty: TinySesam wants at least three segments, as T-33 built them), name and icon on the sign-in page.
- Later TinySesam's permission layer: UnderDoc registers permissions, an admin maps them to roles, the Go client checks has_permission. The two admin levels of T-33 move there.
- An outside gate (a fleet gate in front of several apps) stays possible: then UnderDoc's sidecar is left out, as today.
- Docs: install with local accounts only, with an OIDC provider, mixed; ADR-32 gets an addendum.

**Waits for** TinySesam's TOML configuration (its T-97). The account API contract is stable already; T-33 is built against it before this.
