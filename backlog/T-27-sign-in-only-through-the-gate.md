---
type: Task
title: Sign-in only through the gate
status: erledigt
id: T-27
milestone: M-30
created: '2026-11-23'
---
# T-27 — Sign-in only through the gate

Write and build ADR-32: UnderDoc signs in only through TinySesam (forward auth, gate token, Remote headers) using the gate's Go client pinned by commit. Persons bound by Remote-Id; groups from Remote-Groups as read-only groups (ADR-29, ADR-35); admin from an app role such as underdoc-admin; Remote-Kind and Remote-Auth read from the checked result, Remote-Kind not yet the only barrier (fail closed when the gate announces it). Built-in passkey, password and OIDC sign-in are removed; at most an emergency access on the command line. One tenant; tenant fields stay a placeholder until the gate decides T-73. Header and claim names are configurable.

Read Remote-Auth and Remote-Kind only from the result the gate's Go client has checked. Until this task, T-26 reads Remote-Auth from the header with the same trust as Remote-User (the proxy strips and sets it); the interface for it is `account.User.AuthKind` and `Agent()`. Gate tokens issued before TinySesam 0.26.3 carry no `ts_auth` and count as agents until they expire.

**Done on 2026-11-23** (ADR-32, part 1 and part 2): the gate's Go client checks every request, persons are bound by
`Remote-Id`, the sign-in built in is gone, the emergency access is on the command line. Open for the gate: whether
`Remote-Kind` is the only barrier (then `UNDERDOC_GATE_KIND=required`), the delivery of the gate token to the app (its
T-84, for token mode), and tenants (its T-73).
