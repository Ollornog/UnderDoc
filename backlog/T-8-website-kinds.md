---
id: T-8
type: Task
title: Settle what the site builder covers
status: offen
milestone: M-21
tags: [ui, sites]
created: 2026-11-06
---

The builder is the wanted direction. Still to settle:

- **Sections:** which section and column layouts it offers, tried in mockups.
- **Publishing:** own host name or a path, signed-in or public, and whether UnderDoc serves it or static files go elsewhere.
- **Other websites in a window:** an external site in a frame breaks the rule that nothing is loaded from third parties, and many
  sites refuse framing. Internal tools would be the same mechanism with fewer concerns. Only with an allowlist and a click, or not at all.
- **Hand-written HTML:** needs a separate origin or a sandbox, or its scripts run with the rights of the app. The starting
  recommendation is to leave it out.

**Done when:** the choices are recorded in a decision, and M-21 is updated to match.
