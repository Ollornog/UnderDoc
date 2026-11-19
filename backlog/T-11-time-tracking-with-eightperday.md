---
id: T-11
type: Task
title: Time and leave of the private area with EightPerDay
status: erledigt
milestone: M-24
tags: [module, people, time]
created: 2026-11-19
---

The time clock and the vacation planning of M-24 are [EightPerDay](https://github.com/Ollornog/EightPerDay), embedded as a
library at a pinned version, as settled on 2026-11-16 (privately and for the organization of the tenant at once, the
company features switched on by the company itself). Built on 2026-11-19 ([docs/time-tracking.md](../docs/time-tracking.md)):

- The module Time tracking, off by default, switched by the hoster, the administrator and each user (ADR-25).
- EightPerDay's interface below `/time/`, with UnderDoc's sign-in and in the tenant of the request, shown in a window of
  the workspace from Time and Vacation of the private area.
- Its tables with the prefix `time_` in UnderDoc's database, from its own migrations in UnderDoc's flow, under the guard
  of ADR-23; the door "time tracking" in the separation test of the web.
- The tenant is an organization there, accounts are persons, administrators of the tenant administer it; what a person
  tracks privately only they see.
- `GET /api/time/absences` for the calendar of M-18.
