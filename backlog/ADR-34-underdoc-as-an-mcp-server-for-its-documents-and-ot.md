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

Proposed by the team wiki and UnderDoc on 2026-11-23. The maintainer decides.

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
    UnderDoc. If an agent should be allowed less than its person, UnderDoc needs its own rule based on
    `Remote-Auth: oauth`, or the gate needs finer scopes later. That would be a new wish for the gate.
  - Sensitive actions refuse `Remote-Auth: oauth`, for example deleting for good.
  - Every write and every read of an outside source is logged (ADR-11).
- **No MCP resources and no prompts** for now.
- **Without an AI model on the server:** the MCP server needs no model and works without the AI drawer (M-13).

## Consequences

- UnderDoc carries no OAuth code. The operator lists UnderDoc's address as an OAuth resource in the gate's configuration,
  and the gate registers the MCP clients.
- **Open for the maintainer:** should agents get less than their person, for example read only by default? Either way it
  is decided in UnderDoc first; finer scopes in the gate would be a separate request.
- Sources need a URI scheme and a read and search adapter each (M-4, M-27). Connectors need an extension point with their own
  tools and credentials.
- The search of M-12 has to return sections and rank sources together.
- The tool set is small enough to keep in step with the browser. A tool that the browser cannot do is a sign of a missing
  feature in the interface.
