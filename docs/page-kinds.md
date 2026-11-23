# Page kinds and writing rules

Every page names its kind, and the kind decides who the page is written for (T-20,
[ADR-33](../backlog/ADR-33-ai-documentation-and-human-documentation.md)). Agents may write every kind; their changes to
human documentation stay visible until a person checks them ([Changes by agents](#changes-by-agents)). The rules of each
kind are checked at every write, through every door: the web editor, the API, and later the MCP access (T-21) and the
import (T-28). The rules live once, in `internal/pagekind`; the database runs them at write time.

## Kinds

| Audience | Kinds | Rules |
|---|---|---|
| **Structure and records** | `overview`, `register`, `record`, `plan`, `source` | no kind required; the check looks at structure only |
| **Agent documentation and reference** | `reference`, `agent-notes` | short and dense is fine; one fact in one place; every section has a stable heading or part ID |
| **Human documentation** | `how-to`, `explanation`, `tutorial` | style first: an opening sentence for people, numbered steps, the image at its step, one heading per error; values are embedded, never copied |

- The audience follows from the kind. There is no field of its own for it.
- **A page without a kind counts as human documentation**, the stricter side. It gets a warning, never a refusal.
- A board of quick notes has no kind and no rules.
- Kinds are written in English. The settings of a page offer them in three groups, and `PUT /api/pages/<id>/kind`
  `{"kind": "how-to"}` sets one (empty for none). A new page takes `kind` in `POST /api/pages`, or the kind of its
  template: How-to and Runbook are how-tos, Decision an explanation, Meeting notes a record.
- The kind is written as `kind:` in the front matter of an export, as a board of quick notes says `kind: notes`, and an
  import reads it back ([export.md](export.md)).

### Names an import reads

`pagekind.Parse` reads the English names in any case, with spaces, underscores or hyphens (`How-to`, `agent notes`), and
the German names of a team wiki:

| German | Kind |
|---|---|
| `anleitung` | `how-to` |
| `erklärung`, `erklaerung` | `explanation` |
| `tutorial` | `tutorial` |
| `referenz` | `reference` |
| `agentennotizen`, `agent-notizen` | `agent-notes` |
| `übersicht`, `uebersicht` | `overview` |
| `register` | `register` |
| `datensatz` | `record` |
| `plan` | `plan` |
| `quelle` | `source` |

A team wiki that keeps two axes, `type:` and `art:`, comes to one kind through `pagekind.FromTeamWiki`, for the import
(T-28):

| `type:` | `art:` | Kind |
|---|---|---|
| `Register` | any | `register` |
| `Plan` | any | `plan` |
| `Quelle` | any | `source` |
| `Regeln` or `Thema` | `anleitung`, `referenz`, `erklärung`, `tutorial` | the kind the art names |
| `Regeln` | none | `agent-notes`: rules there are conventions and instructions agents follow, which is what agent notes are; people may still edit them |
| `Thema` | none | none, so human documentation with a warning |

Part addresses of such a wiki name a part as `#tN`; UnderDoc addresses it as `#bN.tN`. The import has to rewrite them.

## The check at write time

Every write of a block runs the rules. **Errors block the write**; **warnings come back with the answer**, each with a
code, the block it is in, what is wrong and how to fix it:

```json
{"code": "COPY", "level": "warning", "block": "b3",
 "message": "The value backup.example.com is copied from Network.",
 "fix": "Embed instead of copying: mark the value as a part there and embed it with :embed{ref=…}.",
 "source": "abcdefghjk#b1.t1"}
```

A refused write answers 400 with `error` and `findings`. The answer of every block that was written
(`POST /api/pages/<id>/blocks`, `PUT …/blocks/<block>`) carries the warnings of the whole page in `findings`. The page
shows them folded above its text to whoever may change it.

| Code | Level | Kinds | What it finds |
|---|---|---|---|
| `KIND` | error | all | a kind that is not one, when it is set |
| `FRONT-MATTER` | error | all | a block that starts with front matter (`---`, a key such as `kind:` or `title:`, `---`) |
| `ID` | error | all | a part ID opened twice in one block |
| `KIND-MISSING` | warning | none | a page with text and no kind |
| `START` | warning | how-to, explanation, tutorial | the first thing is not a paragraph of at least three words |
| `STEPS` | warning | how-to | no numbered list anywhere |
| `COPY` | warning | how-to, explanation | an address, e-mail, URL, path or host name that also stands in a reference or a record the writer reads, with its source |
| `IMAGE-LINK` | warning | human documentation | a link to an image (`[plan](plan.png)`) instead of showing it |
| `ERROR-TABLE` | warning | human documentation | a table whose header names errors, with three rows or more |
| `STABLE-HEADINGS` | warning | reference | no headings or parts in a page of several blocks, or a heading twice |
| `EMBED` | warning | all | an embedding that finds nothing for the writer |
| `DESCRIPTION` | warning | all | a description longer than 120 characters |
| `OVERVIEW` | warning | folders | a folder with two pages or more and no page of the kind overview (list of problems only) |
| `FOLDER-SIZE` | warning | folders | more than 25 pages directly in a folder (list of problems only) |

**Why so little blocks:** by default nothing waits (ADR-33). A page is written block by block, so a rule over the whole
page (an opening sentence, numbered steps) cannot hold while the first block is written. The heuristics (copied values,
error tables) can be wrong, and a person must never be stopped by a guess. An error is only what is broken for certain
and can always be fixed in the same write: a kind that does not exist, front matter in the text where it means nothing,
and a part ID that is no longer unique.

**Copied values:** values are what changes (addresses, ports with their host, paths, mail, URLs). They are compared
with the references and records the writer may read, so a hidden page never shows through a warning. Where the value
stands inside a part, the source is the part, ready to embed. Links to other pages of UnderDoc do not count.

### Codes of the team wiki

| Team wiki | UnderDoc |
|---|---|
| `ART-FEHLT` | `KIND-MISSING` |
| `ID` | `ID` (a part ID twice; page IDs are made by UnderDoc) |
| `ANFANG` | `START` |
| `EINBETTUNG` | `EMBED` |
| `ABSCHRIFT` | `COPY` |
| `BILD-LINK` | `IMAGE-LINK` |
| `UEBERSICHT` | `OVERVIEW` |
| at most 25 pages in a folder | `FOLDER-SIZE` |

## For people and for agents

- **People** see the description on top as a summary, the kind and side under the title, and the text without front
  matter or IDs.
- **Agents** read the raw page: `GET /api/pages/<id>/raw` answers the page as its Markdown file, with its front matter
  (`id`, `kind`, `title`, `description`, `tags` …) and the marker `<!-- block bN -->` before each block, as an export
  writes it. It needs the right to read the page.

## Changes by agents

Every version of a block records whether an agent wrote it (`block_versions.agent`, migration 034). It is taken from
the sign-in of the request as the gate's client checked it (`account.User.Agent`, ADR-32, ADR-36), by a trigger in the
database, never from anything a client sends. The list of versions marks such a change with `(AI)`.

**On human documentation** (how-to, explanation, tutorial, or no kind):

- Every block an agent changed and nobody checked carries the hint **"Written by AI, not checked"** under its text, for
  everyone who reads the page, with whom the agent works for and when. A block an agent made says "Added by AI", and a
  block an agent took out or moved to another page is listed under the page.
- Further changes of an agent extend the hint: what a person sees in the diff is everything since a person last saw
  the block. A block an agent made and took out again before anyone checked it leaves nothing to check.
- Whoever may change the page opens the change as a **diff** (Review) and either sets **Checked** or **takes it back**:
  the text before comes back, a block an agent made goes, a deleted block comes back where it stood with its ID. A block
  an agent moved away is moved back from where it went, so only Checked is offered for it.
- A check names the change it saw (`{"version": n}`); an older change, the change of another block or one an agent
  followed up meanwhile is refused (409), so a click is never blind. Changes are checked one at a time: there is no
  "check all".
- The page shows **"Checked by … on …"** for its last check, and the log keeps each check and each taking back.
- **When a person edits a block themselves, its hint goes**, since they have just read it.
- **An agent never checks**, not even on behalf of its person: checking and taking back refuse every request that is
  not a session (the ceiling of the group Agents, ADR-36). An agent also may not change the kind of a page that people
  wrote to a kind of the agent side, which would hide its hints; a page only agents wrote it may class freely.

**On the agent side** (reference, agent notes) the page says once **"Maintained by AI"** and carries no hint per change.
People may still edit it. Structure pages carry neither.

| Door | What |
|---|---|
| `GET /api/doc/<id>` | `blocks[].ai` (`version`, `by`, `at`, `added`), `removed`, `checked`, `maintainedByAI` |
| `GET /api/pages/<id>/blocks/<block>/change` | the diff: `before`, `now`, `version`; for whoever may change the page |
| `POST /api/pages/<id>/blocks/<block>/check` | `{"version": n}`: Checked |
| `POST /api/pages/<id>/blocks/<block>/takeback` | `{"version": n}`: take back; a block in use needs a way, as a deletion does |
| `GET /api/search?check=ai` | the filter **AI changes to check**: pages with open hints the reader may read, with or without words |

**For the import (T-28):** a team wiki keeps its check as `geprüft: <KÜRZEL> <JJJJ-MM-TT>` in the front matter of a
page. `pagekind.ParseChecked` reads it, and `db.MarkPageChecked` gives the mark to every block of the page and ends its
hints. It has no door in the API, and agents may not call it.

Not yet: a folder that makes changes of agents wait as a proposal (T-35, M-20), and the inbox list (T-36, M-16).

## The list of problems

**Problems** in the workspace lists, next to what does not resolve, every page and folder the viewer sees that breaks a
rule of its kind (`GET /api/problems`, `rules`). It checks at most 2000 pages and folders and reads at most 5000 blocks
of references and records for copied values; `more` says when there was more.
