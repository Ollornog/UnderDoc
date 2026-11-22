---
type: Task
title: Page kinds with writing rules, so pages written by AI read well for people
status: offen
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

Starts with an ADR.
