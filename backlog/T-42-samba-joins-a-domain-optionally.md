---
type: Task
title: Samba joins a domain, optionally
status: offen
points: 8
id: T-42
milestone: M-5
created: '2026-11-24'
---
# T-42 — Samba joins a domain, optionally

Open in M-5 ("Samba can optionally join a domain"). The companion can join an Active Directory domain as a member, so people sign in to the share with their domain account; mapping of domain accounts to UnderDoc accounts through the gate's Remote-Id or a configured attribute. Off by default; without a domain everything stays as today (app password per account). Test against a Samba AD domain controller in a container, synthetic only. Credentials for the join only from a file (ADR-7, ADR-23).
