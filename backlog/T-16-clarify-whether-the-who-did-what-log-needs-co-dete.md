---
type: Task
title: Clarify whether the who-did-what log needs co-determination or GDPR steps
status: erledigt
points: 2
id: T-16
milestone: M-5
created: '2026-11-22'
---
# T-16 — Clarify whether the who-did-what log needs co-determination or GDPR steps

Before running UnderDoc in production, clarify whether the log of who did what and when (ADR-11) is subject to co-determination (works council) and what the GDPR asks for. The answer depends on the country (Germany, Austria). Not legal advice: the result names its sources and what stays open.

**Clarified on 2026-11-24 (research, not legal advice; a lawyer should read the operator page before it ships, above all for Austria):**

- **Germany:** the log is very likely subject to co-determination under § 87 (1) no. 6 BetrVG: "intended to monitor" means objectively suitable and attributable to a person (BAG 13.12.2016, 1 ABR 7/15), whatever the employer means to do. Public service: § 80 (1) no. 21 BPersVG.
- **Austria:** § 96 (1) no. 3 ArbVG only where the system touches human dignity (case by case; then only by works agreement); § 96a ArbVG (personnel data systems) can be replaced by the conciliation board. Without a works council: § 10 AVRAG, consent of each person.
- **GDPR:** legal basis Art. 6 (1) (f), for the version history also (b); § 26 (1) BDSG no longer carries it (BAG 8.5.2025, 8 AZR 209/21, after CJEU C-34/21). No use for performance control; no legal retention period; access under Art. 15 names time and purpose, other employees only by exception (CJEU C-579/21); record of processing (Art. 30); document the DPIA threshold check, a DPIA is likely for complete monitoring.
- What the software should offer follows as T-44.
