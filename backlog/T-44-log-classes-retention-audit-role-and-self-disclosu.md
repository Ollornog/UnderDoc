---
type: Task
title: Log classes, retention, audit role and self-disclosure for co-determination and GDPR
status: offen
sprint: S2
points: 13
id: T-44
milestone: M-31
created: '2026-11-24'
---
# T-44 — Log classes, retention, audit role and self-disclosure for co-determination and GDPR

Follows from T-16 (2026-11-24). What the software offers so an operator in Germany or Austria can run the log properly. Must:

- **Three log classes** with their own retention and roles: version history (part of the content), audit (rights, accounts, invitations, locks, configuration, admin actions, actions of agents on behalf of a person), access and reads (Samba audit, read events).
- **Minimal by default:** reads of people are off; reads of agents only as the state needed for the mark of T-38, not as a history of the person.
- **Retention per class**, with default values as a proposal, automatic deletion or anonymisation, deletion logged.
- **An audit reader role** separate from the administrator; works council or data protection officer as a read-only reviewer; an administrator cannot give themselves audit rights without it showing in the audit.
- **Access to the log is logged**; a query by person needs a reason; optionally a second person approves.
- **No rankings of people**, no activity scores; reports by document or event, not by person.
- **Self-disclosure export** (my log entries, CSV or JSON), actions of other employees blacked out; the export is logged.
- **Notice text** where data is collected, with placeholders (purpose, legal basis, retention, contact), templates in English and German, an Austrian variant.

Should: pseudonymised audit view by default with reveal by reason and four eyes; a generated log profile (events, fields, roles, retention) as an annex for a works agreement and the record of processing; a confirmation switch "works agreement or information in place" before enabling the access class; no shared admin accounts (docs); tamper protection (append only, hash chain) for the audit class; agent actions show the person they act for and the agent separately and are visible to that person.

Operator docs: a page on log, works council and data protection with a checklist, a country table Germany and Austria, points for a works agreement, a DPIA threshold aid and the access process, clearly marked as not legal advice.

**Moved to M-31 on 2026-11-24 (maintainer: later):** needed before the team wiki runs on UnderDoc in a company, not for M-5.
