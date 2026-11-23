# UnderDoc as an MCP server

<p><b>English</b> · <a href="../i18n/docs/mcp.de.md">Deutsch</a></p>

Agents such as coding assistants reach UnderDoc's pages and files over the Model Context Protocol (ADR-34, T-21). They
always act for a person, with that person's rights, and always as an agent (ADR-36). UnderDoc needs no AI model for
this (M-13).

## The endpoint

UnderDoc answers MCP at **`/mcp`**, over the streamable HTTP transport, built on the official Go SDK
(`github.com/modelcontextprotocol/go-sdk`). The server is stateless: every POST is a whole exchange, answered as JSON. It
keeps no session between requests, so each call is decided anew with the rights of the moment. It offers tools only, no
resources and no prompts.

## Sign-in

UnderDoc carries no OAuth code. The gate in front of it (ADR-32, ADR-35) is the OAuth 2.1 authorization server for MCP
clients:

- The operator lists `https://<host>/mcp` as an OAuth resource in the gate's configuration. A token for it holds for that
  path and below it.
- The gate's proxy sends `/.well-known/oauth-protected-resource` of UnderDoc's host to the gate, which writes the
  metadata (RFC 9728). UnderDoc has no handler for it.
- A request without a valid token gets the gate's `401` with `WWW-Authenticate: Bearer resource_metadata=…`. A client
  then signs in through the gate and registers there.
- With a valid token, the gate passes the request on with the same `Remote-*` headers as for a browser, with
  `Remote-Auth: oauth` and `Remote-Kind: user`. The gate's Go client checks them as for every request
  ([sign-in.md](sign-in.md#the-gate)).

A request that reaches `/mcp` without the gate is answered `401`, one without `Remote-Kind` `403`, as everywhere.

To connect a client, give it the address of the endpoint, for example with Claude Code:

```bash
claude mcp add --transport http underdoc https://wiki.example.org/mcp
```

The client finds the gate through the metadata and opens its sign-in in the browser.

## Always an agent

Every call through `/mcp` is an agent's (`account.User.MCP`), also when a browser session of the gate sent it. The
connection of each request to the database learns it, so the one check of rights (`object_decide`) holds the ceiling of
the group Agents ([rights.md](rights.md)):

- no managing rights, no share links, no deleting for good, no passwords;
- a space set to "no agents" is closed, in the tree, in reading and in the search;
- each person can limit their own agents further, for example to reading;
- an administrator's agent administers nothing.

A browser cannot send a request to `/mcp` from a page of another site: such a request (`Sec-Fetch-Site: cross-site`, or
an `Origin` of another host) is refused with `403`, since the gate signs browsers in with a cookie. A program sends
neither and passes.

Every page an agent reads is logged as opened, as in the browser (ADR-11).

## Addresses

The tools name what they read with one scheme, so that other sources can add theirs later (T-31):

| Address | Names |
|---|---|
| `underdoc:<id>` | an object of the database: a page, folder, collection or file |
| `underdoc:<id>#b3` | a block, or the section a heading block opens |
| `underdoc:<id>#b3.t2` | a part (ADR-22) |
| `underdoc:<id>#page=2` | a page of a PDF |
| `underdoc:folder/<id>` | a page of the folder source, read only |

A tool also takes the bare address (`<id>#b3`) and a link of the workspace (`/o/<id>#b3`, with or without host). An
address is only ever looked up by its ID: nothing in it reaches a path on a disk.

## Tools

| Tool | Does |
|---|---|
| `context` | Start here: who the agent acts for and what it may, the rules page, open hints, the tree to a depth (2 unless asked, at most 4, at most 200 entries; `under` for a part of it) |
| `read` | One to five pages as Markdown with front matter, the marker before each block and the version to write against; or one block or section (`#b3`); a page of the folder source as its outline |
| `used_in` | The pages that embed a page, block, section or part |
| `search` | One search over the sections of pages and the pages of PDFs and other files: one line per hit with its address and section and an excerpt, at most 20 hits, the total at the end; error codes exactly |

Answers are compact (ADR-34): a list has at most 20 rows, an answer at most 100 KB. A page larger than that, or larger
than its share when several are read at once, comes as its outline with the address of each section. Errors are answers
the agent reads: "Not found" never tells whether something exists that the person may not see.

### The rules page

The conventions of an instance are one page in the wiki (ADR-33, ADR-34): the oldest page of the kind `agent-notes`
tagged `rules` that the person may read. `context` names it; the instructions of the server say to read it before
writing. People write and change it in the browser like any page.

### Instructions

The server's instructions are short (well below the 4,096 characters clients take): the agent's limits, the addresses,
the audiences of the page kinds, the limits of answers and how to work. Everything else is on the rules page.

## Search

The search returns sections (ADR-24): one record per heading and what follows, one hit per object, codes in fields of
their own and exact, the rights as a filter and each hit checked once more in the database. The text of a PDF keeps its
pages apart, on the server and in the browser, so each page is records of its own and a hit names its page
(`#page=3`). A word takes a typing error from four letters on (T-24, measured in [search.md](search.md)).

## What is not here yet

- Writing, moving, the trash and files come in the second part of T-21.
- Other sources (`source_search`, `source_read`) and the tools of connectors come with T-31.
- Automatic sections (T-30) do not exist yet, so `read` has nothing to count.
