---
type: Decision
title: UnderDoc as an MCP server for its documents and other sources
status: erledigt
id: ADR-34
tags: [ai, mcp, sources]
created: '2026-11-23'
---
# ADR-34 — UnderDoc as an MCP server for its documents and other sources

## Context

The maintainer wants UnderDoc to become an MCP server itself. Agents such as coding assistants should reach its documents
and also the sources UnderDoc connects (file shares, spreadsheets, Git, outside systems, M-4, M-27). They always work
under the rights of the person they act for. T-21 lists what agents need for pages. M-13 keeps AI on the server optional.

A team wiki with about 70 MCP tools logged some 10,800 calls in four days:

- **Most calls:** reading, writing, the index, search, move, replace, set front matter, file upload.
- **Almost never:** history, versions, parts, collections, groups and admin tools. People do these in the browser.
- **What helped agents most:** feedback in the answer of a tool, for example a lint warning with a fix, "file nobody links
  to", "link to nowhere", "page changed since you read it".
- **What was missing:** reading several pages in one call; a search that returns sections with anchor and excerpt and
  ranks pages and PDF text together; a dry run for writing; one call that gives an agent its start context; "used in" as a
  tool.
- **Conventions:**
  - The MCP instructions hit their limit of about 4,096 characters several times.
  - Rules written in three places (instructions, skill, wiki page) contradicted each other.
  - Versions of the skill drifted apart between colleagues.
- **Resources and prompts** in the MCP sense were not used. Clients work with tools.

Sign-in is settled by the gate (ADR-32, ADR-35): it is the OAuth 2.1 authorization server for MCP clients, and its forward
auth turns a bearer token into the same `Remote-*` headers as a browser session, with `Remote-Auth: oauth`.

## Options

**How many tools:**

- **One tool per function,** as the team wiki grew: complete, but most tools stay unused, and the list costs every agent
  context.
- **A small core plus tools that appear only where they apply.**

**How agents reach other sources:**

- **Tools per source:** each connector brings its own read, search and act tools. That makes many tools, and each one
  checks rights in its own way.
- **One way to read and search all sources, and acting per source:** a common search and a common read by URI, one rights
  check; tools that change something in an outside system come per connector.

**Where the conventions live:**

- **In the MCP instructions:** always present, but short and hard to keep in step.
- **In a skill package:** good for setting up a client, but versions drift.
- **As a page in the wiki,** pointed to by the instructions and enforced by the check at write time.

## Decision

Proposed by the team wiki and UnderDoc on 2026-11-23. Accepted by the maintainer on 2026-11-23.

**The core tools:**

| Group | Tools |
|---|---|
| Start | **context**: topic tree to a depth, the rules page, the caller's profile and rights, open hints for them |
| Read | read one or several pages, or one section; automatic sections only as a count unless asked; "used in" |
| Search | one search over pages, files and connected sources: sections with anchor and excerpt, facets by source, synonyms |
| Write | write a page or a part against a base version; replace a passage; **dry run** that returns the check and the diff without saving; set front matter |
| Structure | move with a lasting redirect; archive instead of delete; files: list, read a page of a PDF as text or image, upload through a one-time link |

- **Answers teach:**
  - Every write answers with the result of the check: errors block, warnings come with a fix.
  - The hints are "file nobody links to", "link to nowhere" and "page changed since you read it".
- **Other sources:**
  - Search and read go one way, `source_search` and `source_read(uri)`. A URI scheme per source, for example a file, a
    share path, a sheet and range, a Git path at a revision, a record of an outside system.
  - One rights check runs for all of them.
  - Tools that act in an outside system come per connector, as an extension. They appear only for someone with access to
    that connector. Credentials are shared or per person and stored encrypted. Administrators choose which connector tools
    a group gets.
- **Conventions:**
  - The rules are a page in the wiki (single source).
  - The instructions are short and point to it.
  - The check at write time enforces it (ADR-33).
  - A skill only sets up the client and describes behaviour, for example "move the work forward first, then ask whether
    and where to save". The server offers its version, and updating needs the person's consent.
- **Limits:**
  - Compact answers: tables at most 20 rows, the tree to a depth, PDF text page by page.
  - Pages at most 25 KB and descriptions at most 120 characters, both checked.
  - Large files only through a one-time link or a direct upload, never base64 in a message.
  - Timeouts for connectors.
- **Rights and log:**
  - Every call runs as the person behind the token, with the same checks as the browser.
  - The gate's OAuth scopes are only `mcp` and `offline_access`, so an agent's token carries the person's full rights on
    UnderDoc. UnderDoc therefore limits agents itself, with the built-in group Agents of ADR-36: no managing rights, no
    share links, no deleting for good, no passwords. Spaces can be set to "no agents", and every person can limit their own
    agents.
  - Every write and every read of an outside source is logged (ADR-11).
- **No MCP resources and no prompts** for now.
- **Without an AI model on the server:** the MCP server needs no model and works without the AI drawer (M-13).

**After review by the team wiki (2026-11-23):**

- A search result is compact: one line per hit with where it is, a short excerpt, and the total count at the end.
- The team wiki's search on Meilisearch, built along M-12, is a reference:
  - one document per section, distinct per object, title and heading also whole;
  - codes in a field of their own;
  - rights as a filter on the space, then the exceptions per folder;
  - PDF pages as documents of their own, the source as a facet;
  - sync per space when its state changes, then the old state is removed by filter.
  - Measurably better: word parts, typos and the jump to the section.

## Consequences

- UnderDoc carries no OAuth code. The operator lists UnderDoc's address as an OAuth resource in the gate's configuration,
  and the gate registers the MCP clients.
- Sources need a URI scheme and a read and search adapter each (M-4, M-27). Connectors need an extension point with their own
  tools and credentials.
- The search of M-12 has to return sections and rank sources together.
- The tool set is small enough to keep in step with the browser. A tool that the browser cannot do is a sign of a missing
  feature in the interface.
- **Built on 2026-11-23, part 1** ([docs/mcp.md](../docs/mcp.md), T-21, T-24). Settled in the build:
  - The official Go SDK (`github.com/modelcontextprotocol/go-sdk`, v1.8.0, MIT and Apache-2.0) carries the protocol, in
    its stateless mode with JSON answers: each request makes its own server, so every tool sees only the person, tenant
    and connection of that request. The path is fixed, `/mcp`; the gate serves `/.well-known/oauth-protected-resource`
    for it, so UnderDoc has neither a handler nor a setting for OAuth.
  - Every call through MCP is an agent's, also a browser session sent there (`account.User.MCP`), and makes no space
    and administers nothing. A browser's cross-site request is refused, as the gate signs browsers in with a cookie.
  - The addresses are `underdoc:<id>` with the fragments of ADR-18, `#page=<n>` for a page of a file and
    `underdoc:folder/<id>` for the folder source, so other sources add schemes of their own (T-31).
  - The rules page is the oldest page of the kind agent-notes tagged `rules` the person reads; no setting.
  - The version to write against is the newest change of the page's blocks; moving a block inside the page makes none.
  - The search of M-12 already kept one record per section, codes in fields of their own, the rights as a filter and
    one hit per object. New: the pages of a PDF are records of their own; the type stands in for the source facet until
    other sources come (T-31). A word takes a typing error from four letters on (T-24).
  - Automatic sections (T-30) do not exist yet, so `read` has no count of them.
- **Built on 2026-11-23, part 2** ([docs/mcp.md](../docs/mcp.md#writing), T-21, migration 036). Settled in the build:
  - A write names the part it replaces: the whole page, or the section a block opens. Blocks are matched by the IDs of
    their markers within that part only; a marker without an ID makes a new block, and an ID from elsewhere is never
    taken. The whole write is one transaction against the version read; a dry run is that transaction undone.
  - Front matter in a write is not applied; title, tags and kind have a tool of their own. The description has no way
    in the browser, so none in MCP either (a gap of the interface, noted in docs/mcp.md).
  - "Link to nowhere", "file nobody links to" and a page above 25 KB are warnings of the one check (`LINK`,
    `UNLINKED-FILE`, `PAGE-SIZE`), so the browser shows them as well.
  - Archive instead of delete is the trash: an agent moves to it, a person restores or deletes for good.
  - The one-time upload link reuses the upload of the browser: the agent starts it with its person's rights, the link
    holds its hash, and a program sends the file with one `PUT` below `/s/`, which the gate already leaves open for share
    links, so the gate needs no new rule. It is no share link: it reads nothing, works once and for one file.
  - Moving a block inside a page makes no version, so it carries no hint of AI (the rest of T-20 stays open).
