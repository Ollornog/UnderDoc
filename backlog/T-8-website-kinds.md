---
id: T-8
type: Task
title: Decide which kinds of website UnderDoc shows
status: offen
milestone: M-21
tags: [ui, security]
created: 2026-11-06
---

- **External site in a frame:** breaks the rule that nothing is loaded from third parties, many sites refuse framing, and it
  tells the other site who is reading. Only with an allowlist and a click.
- **Internal tools in a frame:** the same mechanism, with fewer of those concerns.
- **Static HTML from a folder:** needs a separate origin or a sandbox, or uploaded scripts run with the rights of the app.
- **Link with a preview:** the preview is fetched by the server, which needs protection against SSRF.

**Done when:** the kinds are chosen and recorded in a decision, and M-21 is updated to match.
