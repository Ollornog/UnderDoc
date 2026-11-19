# Time tracking

The module **Time tracking** (`time`, off by default, [docs/modules.md](modules.md)) is
[EightPerDay](https://github.com/Ollornog/EightPerDay), embedded as a Go library at a pinned version (`go.mod`): its
core keeps the data, and its own web interface runs inside UnderDoc. Nothing of it is copied into UnderDoc.

## Where it runs

- Its interface answers below `/time/` (`internal/web/timetracking.go`) while the module is on for the tenant; off,
  every address of it answers 404, `/api/time/…` included, and nothing is deleted.
- It runs with UnderDoc's sign-in: whoever is signed in to the workspace is signed in there, with no sign-in, account
  or administration page of its own. Without UnderDoc's sign-in nothing of it answers.
- It runs in the tenant of the request: its store works on UnderDoc's pool, whose connections carry
  `underdoc.tenant`, so row-level security keeps the tenants apart as for every other table.
- Its pages may be framed by pages of the same origin only (`frame-ancestors 'self'`); UnderDoc's pages allow frames
  of their own origin (`frame-src 'self'`). The pages follow the theme of the workspace (`ud.theme`).

## In the workspace

With the module on, **Time** and **Vacation** in the private area of nav1 are the time tracking; with it off they are gone
from nav1. Their list in nav3 names the pages of EightPerDay (list, every week, the week by mode, all employers, reports
and export, organizations; leave planning under Vacation), and each opens like a document in the active window. The
window shows the page in a frame of the same origin, in which everything of EightPerDay works as on its own, its tab bar
included. The workspace draws its windows anew as a whole, so the frame lives outside them, one per window, laid over
the place its tab keeps free (`assets/app/timetracking.js`); a redraw of the workspace leaves it, and what is typed in
it, as it was. The browser test `TestTimeTrackingInTheWorkspace` switches the module on, records a week in the frame by
mouse and keyboard and checks that another tenant sees nothing of it.

## Tenants, organizations and persons

- **The tenant is an organization** in EightPerDay, its ID the tenant's and its name the tenant's name
  (`db.MapTenant`, kept up to date on the way in).
- **An account is a person**: its ID where EightPerDay takes it (letters, digits, `-` and `_`, up to 64), otherwise
  `a-` and a hash of it that stays the same (`db.Person`).
- **An administrator of the tenant administers the organization**: the mapping gives them an administrator's
  membership once; it never takes one away. Who else belongs to the organization, and who administers it besides, its
  administrators decide on its page in the time tracking, choosing among the accounts of the tenant.

## Private and managed at once

- **Private**: what a person tracks for employers, clients or work of their own is theirs alone. Nobody else sees it,
  administrators of the tenant included; the organization reads only its managed employers (`Store.Managed`).
- **Managed**: a member of the organization records the time of their employment with it on a managed employer. Its
  administrators see that part by EightPerDay's rules: members, schedules, pay, the company features (approval, a
  supervisor's view), which are off unless switched on with the reference to a works agreement, and every read of a
  member's data goes into that member's access log.
- A person's overview over every employer, with its hints on the Working Hours Act, is shown only to the person.

## Database

- EightPerDay's migrations run in UnderDoc's own flow after UnderDoc's, rendered with the prefix `time_` and the
  setting `underdoc.tenant` (`db.TimeOptions`), as versions from 1001 in `schema_migrations`. Each table then gets a
  reference to `tenants`, as every table of a tenant has one. UnderDoc's own migrations never touch these tables.
- The guard of ADR-23 (`TestEveryTableKeepsTenantsApart`) reads them like every other table and fails if fewer than six
  are found; `TestNoTableShowsAnotherTenant` fills them, and `TestTenantsSeeNothingOfEachOther` goes through the door
  "time tracking" of the web as well.
- The access log of the time tracking lets go of a row only after two years. Deleting a tenant for good
  (`PurgeTenant`) sets `underdoc.tenant_erase` to that tenant in its transaction, which lets that one tenant's rows go.

## For the calendar

`GET /api/time/absences?from=2026-01-01&to=2026-12-31` lists the absences of the signed-in user over all their
employers, private and managed, as `core.CalendarAbsences` gives them: type and its name, employer, days, half days,
status, whether they count as leave and whether the type reveals health data, never a note. At most two years at a
time. It is the interface for the calendar of M-18; the calendar itself does not exist yet.
