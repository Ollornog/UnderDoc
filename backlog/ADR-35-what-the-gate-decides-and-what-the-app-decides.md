---
type: Decision
title: What the gate decides and what the app decides
status: offen
id: ADR-35
tags: [permissions, sign-in, tenants]
created: '2026-11-23'
---
# ADR-35 — What the gate decides and what the app decides

## Context

UnderDoc will sign in only through the gate, TinySesam (ADR-32). ADR-29 settled roles on groups and persons. Until now the
split was not settled: which rights the gate holds, and which the app holds. The maintainer set the line on 2026-11-22:
the gate gives identity, coarse roles, ways to sign in and approvals. Fine rights on objects of an app belong to the app.

A team wiki behind the same gate learned the following:

- Its people were confused by "teams" next to the gate's groups, moderators next to leads, and old department roles from
  the sign-in next to the rights in the app.
- What works now:
  - persons and groups;
  - groups inside groups, passed down by default;
  - rights manage, write and read on a space for persons or groups, where the highest right counts;
  - group admins who manage their part of the tree;
  - one page per space and per group, with the danger zone folded away.
- The most useful rule was not a right but a filing rule: knowledge for everyone lives in the open with tags, and spaces
  hold only what is internal to a team or sensitive.
- An exception per folder was needed once: a feedback inbox everyone may write to.

The state of the gate on 2026-11-23:

- The gate calls groups "roles". They arrive in `Remote-Groups` or the gate token. A role is never renamed and is deleted
  only when unused.
- Accounts carry a lasting ID in `Remote-Id`.
- `Remote-Kind` tells a person from a service account. It is merged and comes with gate version 0.28.0.
- `Remote-Auth` says how someone signed in: `session`, `oauth`, `apikey` or `app-password`.
- An account API lets an app with its own service account and an allowlist of roles invite, change, lock and reset
  accounts, always on behalf of a person.
- Approvals with a step-up exist for the gate's own actions. Signed one-time approvals for actions of an app come next.
- Tenants (T-73 of the gate) are not built yet.

## Options

- **Rights as roles in the gate:** owners and moderators of each space become gate roles. The maintainer rejected this on
  2026-11-22, because the gate would have to know every object of every app.
- **Rights only in the app, the gate only for sign-in:** the app has to rebuild invitations, locking and approvals.
- **The line of the maintainer:** the gate holds who someone is, coarse roles, how they signed in, the account lifecycle
  and approvals. The app holds everything about its objects.

## Decision

The line of the maintainer. Proposed by UnderDoc, the gate and the team wiki on 2026-11-23. The maintainer decides.

**The gate decides:**

- Who someone is: a person is bound by `Remote-Id`, never by name or mail.
- Whether someone may use UnderDoc at all: the gate's role for the app.
- The coarse roles UnderDoc reads:
  - administrator of the tenant;
  - may create spaces;
  - superadmin of the gate.
  The gate's own admin flag is never a right in UnderDoc.
- Person or service account (`Remote-Kind`). Once gate 0.28.0 has rolled out and old tokens have expired, a request without
  it is refused.
- How someone signed in (`Remote-Auth`). Sensitive actions can demand a session.
- The account lifecycle through the account API: invite, lock, unlock, new link, reset with approval, later delete with a
  grace period.
- Approvals for dangerous actions, for example deleting a space for good, once the gate signs approvals for apps.
- Later the tenant (`Remote-Tenant`, open until the gate decides T-73). Until then UnderDoc runs one tenant (ADR-23).

**UnderDoc decides:**

- Roles on objects as in ADR-29: owner, manage, edit, comment and read, for persons and groups, down to the single object.
- **Groups:** groups taken over from the gate's roles can only be read, as in ADR-29, and are never copied. Local groups
  are made in UnderDoc.
  - **Open for the maintainer: groups inside groups.** ADR-29 said no for now. The team wiki shows that a tree of groups
    with admins per part carries a company well. Recommendation: allow nesting for local groups, passed down by default;
    roles of the gate stay flat.
- Group admins in UnderDoc invite colleagues and lock them. UnderDoc calls the account API with its service account, on
  behalf of the person who clicked, and only with roles on its allowlist.
- What a service account may reach: the gate gives it roles. UnderDoc grants it paths or spaces like any person, for example
  an export that may only replace one data page.
- Exceptions per folder only as an explicit special case, for example an inbox everyone may write to.
- **Filing rule** as a default the interface suggests: knowledge in the open with tags, spaces for what is internal or
  sensitive.
- Clear names in the interface: "group" for both kinds, with a mark for where each comes from. There are no "teams" next to
  groups.

## Consequences

- ADR-32 uses the gate's Go client for the gate token and the headers, pinned by commit, and binds persons by `Remote-Id`.
- The account API needs a service account of UnderDoc and an allowlist in the gate's configuration.
- Nesting local groups changes ADR-29, if the maintainer agrees.
- Tenants stay a placeholder until the gate has decided T-73.
