---
type: Decision
title: AI documentation and human documentation
status: erledigt
id: ADR-33
tags: [ai, pages, review]
created: '2026-11-23'
---
# ADR-33 — AI documentation and human documentation

## Context

A team wiki that people and AI agents have written together for some weeks shows the following:

- **Agents write dense reference notes.** They are complete and easy to search, but hard to read. Typical faults:
  - abbreviations and unexplained jargon
  - copied values
  - long error tables
  - images linked instead of shown at the step
  - every intermediate state of a discussion saved
- **People mostly read how-to guides with images, error pictures, lists of ports and customer pages.** What bothers them most
  is missing structure, not the text: tree, breadcrumbs, folder overview, "used in", table of contents.
- **The wiki has no human review.** A mandatory review would leave pages waiting and let them go stale.

UnderDoc wants documentation that reads as if people wrote it, while agents do much of the writing.

## Options

- **One kind of page, writing rules only:** simple, but readability depends on discipline.
- **A generated human view** of the reference: only one place to maintain, but it drifts or invents, and nobody checks it.
- **Two apps,** one for agents and one for people: a lot of friction at the border (rights, IDs, embedding across
  systems, sync, search).
- **Every AI change waits for approval:** safe, but the documentation piles up unchecked and goes stale.
- **Two audiences in one source.** Agents write the agent side freely and give style priority on the human side. Their
  changes to human pages are visible and can be checked, without blocking by default.

## Decision

The last option, settled with the maintainer and the team wiki on 2026-11-23.

**Every page has a kind (T-20), and the kind decides its audience:**

| Audience | Kinds | Who writes | Rules |
|---|---|---|---|
| **Agent documentation and reference** | reference, record, agent notes (conventions and instructions for agents) | agents may write and change it completely | short and dense is fine; one fact in one place; every section has a stable heading or part ID so it can be embedded |
| **Human documentation** | how-to, explanation, tutorial | people and agents | style comes first: an opening sentence for people, numbered steps with the why, "if it does not work", jargon explained, the image at its step, one heading per error; values are only embedded from the reference, never copied |

**Changes by agents are visible:**

- Every version records whether a person or an agent wrote it (ADR-11). An agent acts on behalf of a person or as a service
  account, signed in through the gate (ADR-32).
- On human documentation, every block an agent changed carries a small hint "written by AI, not checked". Readers see it.
- A person with edit rights opens the change as a diff and either checks it ("Checked") or takes it back. A check records
  who and when, and the page shows "checked by … on …".
- Changes are checked one at a time, with the diff on screen. There is no "check all" across pages, so a click is
  never blind.
- When a person edits a block themselves, the hint on that block goes away, because they have just read it.
- An inbox list "AI changes to check" (M-16) and a search filter collect the open hints.
- **By default nothing waits:** an agent's change is live at once, with its hint. A folder can be switched to reviewed
  (M-20): then agent changes to human documentation wait as a proposal until someone checks them. The agent side never
  waits.
- The agent side shows once per page that it is maintained by AI, and has no hint per change. People may still edit it.

**Agents learn the rules from the server:**

- The MCP instructions (T-21) name the audiences and the rules of every kind.
- The check at write time blocks errors and returns warnings with a hint how to fix them, for example "embed instead of
  copying" or "show the image at its step".

**People get structure first:** tree, breadcrumbs, folder overview, "used in", and a table of contents that marks where
the reader is. Front matter and IDs stay hidden; the description shows on top as a summary (M-14, M-30).

## Consequences

- The page kind becomes mandatory. A page without a kind counts as human documentation, which has the stricter rules.
- Versions need the kind of author (person or agent), and blocks need a mark for "changed by AI, not checked".
- The check needs a set of rules per kind. The team wiki's rules are the starting point: copied value, image only linked,
  page that starts with a list, too long a description.
- If the inbox is ignored, hints pile up. They stay visible and never expire on their own, so they are not lost either.
