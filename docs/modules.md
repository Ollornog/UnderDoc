# Modules

Every module of UnderDoc can be switched on and off ([ADR-25](../backlog/ADR-25-modules-and-profiles.md)). Today there
are three: **Documents** (collections, folders and pages with comments, embeddings, tags, attachments, the trash and the
folder source), **Files** (the view Files of [docs/files.md](files.md): its roots, files with versions, uploads) and
**Time tracking** (EightPerDay below `/time/`, [docs/time-tracking.md](time-tracking.md)), the only one off by
default. The modules of the later milestones join them.

## Who switches what

| Who | Where | What |
|---|---|---|
| Whoever runs the instance | `UNDERDOC_MODULES` | Which modules are allowed at all, and which are on by default. |
| The hoster | Hoster administration, per tenant | Which of those a tenant may use (default: all). |
| The administrator of a tenant | Profile › Modules › Switch modules (`/admin/modules`) | Which of those are on for the tenant. |
| Each user | Profile › Modules › Hide modules for me (`/account/modules`) | Which of those that are on they do not want to see. |

`UNDERDOC_MODULES` names modules separated by commas, each allowed and on by default, or with `:off` allowed but off by
default; a module not named is not allowed. Empty allows every module with its own default. An unknown name stops the
start.

```bash
UNDERDOC_MODULES=documents          # Documents on; Files not allowed
UNDERDOC_MODULES=documents,files:off # both allowed, Files off until a tenant switches it on
UNDERDOC_MODULES=documents,files,time:off # all three allowed, Time tracking off until a tenant switches it on
```

## What off means

- **Nothing is deleted.** Switched on again, a module has everything back as it was.
- **No route:** every address of the module answers 404, as one that never was, the API included.
- **No entry:** the workspace shows nothing of it (`/api/state` lists no collections without Documents).
- **No job:** its background work does not run for that tenant (Documents: the trash does not empty itself; Files: no
  versions are thinned, no blobs collected, no uploads expire).
- **Hidden is not off:** what a user hides is gone from their workspace only; addresses and embeddings of it still
  answer, and the others see it.

`/api/state` lists every module with `allowed`, `on` and `hidden`.

## A new module

1. Add it to `module.All` (package `internal/module`) with its ID, name, description and default.
2. Register its routes through the guard of the web (`h.guard(id, handler)`), and let its jobs ask
   `web.ModuleOn(database, policy, id)` per tenant.
3. Leave out its entries from `/api/state` where it is not shown.
4. Test it with the module on and off: routes, entries, jobs, and that nothing is lost.
