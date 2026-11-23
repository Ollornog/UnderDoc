---
type: Task
title: Page kinds with writing rules, so pages written by AI read well for people
status: erledigt
id: T-20
milestone: M-1
created: '2026-11-22'
---
# T-20 — Page kinds with writing rules, so pages written by AI read well for people

Pages written by AI agents tend to be dense reference notes: complete and easy to search, but hard for people to read. People mostly look for how-to guides and explanations. Observed in a team wiki that people and AI agents write together.

One source, not two wikis (ADR-8):

- A page names its kind, following Diátaxis: tutorial, how-to guide, reference or explanation.
- Each kind has writing rules for people and agents. A how-to starts with what it is for and numbers its steps; an explanation says why; a reference stays short.
- People see a short summary on top, and the machine parts (front matter, IDs) are hidden. Agents read the raw page with its front matter.
- Facts that are really records (customers, cases, persons) live in database tables (ADR-27), and pages show them; they are not copied into prose.
- A check points out pages that break the rules of their kind, for example a page made only of bullet points without an opening sentence.

Kinds share facts, they do not copy them:

- **Every fact lives in one place.** Values that change (addresses, ports, paths, roles, limits, contacts) live in a reference or a record. A how-to or an explanation embeds the part (`:embed{ref=…}`, `::embed{ref=…}`, ADR-22) or links to it, and never writes the value down again. A changed value is changed once; a changed procedure only in the how-to.
- **Before changing a part, "used in" shows** which guides embed it (M-3).
- **The check also finds copied values:** a value written in a how-to or an explanation that also stands in a reference or a record is reported as "embed instead of copying", naming the source.
- **Small topics stay one page** with sections in this order: what it is for, the steps, the values, the background. Separate pages pay off only when several guides use one reference, or a guide grows long.
- Write the reference first; a how-to only for tasks that come up often. Not every topic needs all four kinds.

Decided in ADR-33: the kind also sets the audience, agent documentation and reference or human documentation, and changes by agents to human documentation stay visible until a person checks them.

**Done 2026-11-23** ([docs/page-kinds.md](../docs/page-kinds.md)): kinds with their rules checked at every write (part
1), and the hints of changes by agents with the diff, Checked and taking back (part 2); ADR-33 "Built on" says what was
settled. Follow-ups: T-35 (folder mode reviewed), T-36 (inbox list).
