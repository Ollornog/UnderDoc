---
type: Task
title: Samba joins a domain, optionally
status: erledigt
points: 8
id: T-42
milestone: M-5
created: '2026-11-24'
---
# T-42 — Samba joins a domain, optionally

Open in M-5 ("Samba can optionally join a domain"). The companion can join an Active Directory domain as a member, so people sign in to the share with their domain account; mapping of domain accounts to UnderDoc accounts through the gate's Remote-Id or a configured attribute. Off by default; without a domain everything stays as today (app password per account). Test against a Samba AD domain controller in a container, synthetic only. Credentials for the join only from a file (ADR-7, ADR-23).

**Decided by the maintainer on 2026-11-24:** with a domain, only domain accounts reach the share, no local app passwords (idmap_nss maps by name, so a local name could meet a domain name). The mode is chosen when the share is set up (local app passwords or domain), never mixed; changing it later is an explicit migration by the operator, not a switch at run time.

**Built on 2026-11-26** as in [ADR-30](ADR-30-samba-share.md#settled-on-2026-11-26-samba-joins-a-domain-optionally-t-42)
and [docs/samba.md](../docs/samba.md#a-domain-optional): one mode per tenant, chosen at setup; the companion joins from a
password file and maps domain accounts to the UIDs of their Samba users by name; the account turns SMB on itself, under
the name of its domain account. The maintainer's acceptance against a real domain is open.
