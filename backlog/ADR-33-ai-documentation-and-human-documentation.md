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

The last option, worked out with the maintainer and the team wiki and accepted by the maintainer on 2026-11-23, together
with the concept round (ADR-34, ADR-35, ADR-36, M-30).

**Every page has a kind (T-20), and the kind decides its audience:**

| Audience | Kinds | Who writes | Rules |
|---|---|---|---|
| **Structure and records** | overview, register, record (customer, case, person), plan, source | people and agents | no kind required, no AI hint; the check looks at structure only |
| **Agent documentation and reference** | reference, agent notes (conventions and instructions for agents) | agents may write and change it completely | short and dense is fine; one fact in one place; every section has a stable heading or part ID so it can be embedded |
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

**How agents work:** these rules stand on the rules page and are taught by the server.

- Do not save every intermediate state. Move the work forward first, then ask actively whether and where to save it.
- Choose the place, tags and names yourself, and ask the person only about facts.
- Report an unclear rule to a feedback inbox for agents instead of asking the person.
- **Filing:** knowledge for everyone goes into the open with tags; spaces only hold what is internal to a team or sensitive
  (ADR-35).

**Agents learn the rules from the server:**

- The MCP instructions (T-21) name the audiences and the rules of every kind.
- The check at write time blocks errors and returns warnings with a hint how to fix them, for example "embed instead of
  copying" or "show the image at its step".

**People get structure first:** tree, breadcrumbs, folder overview, "used in", and a table of contents that marks where
the reader is. Front matter and IDs stay hidden; the description shows on top as a summary (M-14, M-30).

## Consequences

- The page kind becomes mandatory for prose. Structure pages and records need none. A prose page without a kind counts as
  human documentation, which has the stricter rules. An import therefore does not flag overviews, registers or records.
- Versions need the kind of author (person or agent), and blocks need a mark for "changed by AI, not checked".
- The check needs a set of rules per kind. The team wiki's rules are the starting point: copied value, image only linked,
  page that starts with a list, too long a description.
- If the inbox is ignored, hints pile up. They stay visible and never expire on their own, so they are not lost either.
- **Built on 2026-11-23, part 1: kinds and the check** ([docs/page-kinds.md](../docs/page-kinds.md), migration 033,
  T-20). Settled in the build:
  - The kind is a field of the page (`kind:` in the front matter of an export), one of overview, register, record, plan,
    source, reference, agent-notes, how-to, explanation, tutorial, or none. The audience follows from it.
  - **A page without a kind is never blocked.** It counts as human documentation and gets the warning `KIND-MISSING`.
    "The page kind becomes mandatory for prose" above means this warning, not a refusal.
  - **Errors block only what is broken for certain and can be fixed in the same write:** a kind that is not one
    (`KIND`), front matter in the text of a block (`FRONT-MATTER`), a part ID opened twice in a block (`ID`). Everything
    else is a warning with its fix: a page is written block by block, so rules over the whole page cannot hold while it
    grows, heuristics such as copied values can be wrong, and by default nothing waits.
  - The rules sit once in package `pagekind`; the database refuses the errors at every write of a block and gathers the
    warnings for the answer of the write, the page and the list of problems, so the MCP access (T-21) and the import
    (T-28) reach the same check.
  - Copied values (`COPY`) are addresses, e-mail, URLs, paths and host names, compared only with the references and
    records the writer may read, so a warning never shows a hidden page.
  - The German values and the two axes of the team wiki (`type:` and `art:`) map onto the one kind; `Regeln` without an
    art becomes agent notes. Its lint codes map onto English codes (table in docs/page-kinds.md).
  - People get the description on top, the kind and side under the title and no front matter or IDs; agents read the raw
    page at `GET /api/pages/<id>/raw`.
- **Built on 2026-11-23, part 2: changes by agents** ([docs/page-kinds.md](../docs/page-kinds.md#changes-by-agents),
  migration 034, T-20). Settled in the build:
  - Whether an agent wrote a version comes from the sign-in of the request (`account.User.Agent`): a trigger takes it
    from the connection, which the application sets per request, so no client and no code path can claim otherwise.
  - The hint belongs to the block and spans every unchecked change of agents since a person last saw it: the diff shows
    all of it, and a check names the newest change, so one made while the diff was open is never checked blind.
  - A check is a person's: checking and taking back refuse every request of an agent, as the ceiling of the group Agents
    (ADR-36), and need the right to write. Taking back restores the text before, removes a block an agent made and puts
    a deleted block back where it stood; a block an agent moved to another page is moved back from there.
  - Moving a block out, deleting it and adding one are changes like editing: what an agent takes out of human
    documentation is listed under the page until checked.
  - An agent may not change the kind of a page people wrote to a kind of the agent side, which would hide its hints; a
    page only agents wrote it may class freely. Hints on a page whose kind a person changes to the agent side stay
    kept and show again if it comes back.
  - The page shows its last check, "checked by … on …". The import of a team wiki marks every block of a page checked
    with its `geprüft:` (`db.MarkPageChecked`), a door without an API.
  - The search filter "AI changes to check" is the database's list of pages with open hints, passed to the search
    engine as a filter of objects, so it is current at once. The inbox list (M-16) is T-36, the folder mode reviewed
    (M-20) is T-35.
