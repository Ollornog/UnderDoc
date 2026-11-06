---
id: ADR-9
type: Decision
title: TinySesam as the sign-in gate
status: erledigt
tags: [auth]
created: 2026-11-06
---

## Context

OIDC, SAML and LDAP with group mapping are needed, and writing them is no reason to build this project.

## Options

- **Gate in front (TinySesam):** existing, independent of language, covers all protocols.
- **OIDC inside the app:** less moving parts, but no SAML or LDAP without more work.

## Decision

TinySesam as a gate in front of the app, passing user and groups through trusted headers.

## Consequences

- The app must only be reachable through the gate.
- Public share paths stay open at the gate and are protected by their own token.
- File access cannot use this sign-in, see ADR-10.
