---
type: Milestone
title: 'TinySesam built in: UnderDoc brings its own gate with local accounts and OIDC'
status: offen
sprint: später
rest_points: 3
rank: 8
id: M-32
created: '2026-11-24'
---
# M-32 — TinySesam built in: UnderDoc brings its own gate with local accounts and OIDC

Split off from M-30 on 2026-11-24 by the maintainer. TinySesam is always built into UnderDoc as a gate right in front of the app; local accounts and OIDC providers run mixed (T-39). Waits for TinySesam's server mode with TOML configuration (TinySesam M-4, T-97) and its permission layer (T-100, T-101).

**Done when this is true:**

- UnderDoc's compose file brings a TinySesam sidecar; an install works with local accounts only, with an OIDC provider, or mixed (T-39).
- The account API, approvals and the sign-in page are configured through apps.underdoc.
- The two admin levels use TinySesam's permission layer.
- An outside gate in front of several apps stays possible.
