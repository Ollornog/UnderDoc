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
| `context` | Start here: who the agent acts for and what it may, the rules page, open hints, the tree to a depth (2 unless asked, at most 4, at most 50 entries, then it points to `under` for a part of it, B-5); archived spaces only with `archived` |
| `read` | One to five pages as Markdown with front matter, the marker before each block and the version to write against; or one block or section (`#b3`); a page of the folder source as its outline |
| `used_in` | The pages that embed a page, block, section or part |
| `search` | One search over the sections of pages and the pages of PDFs and other files: one line per hit with its address and section and an excerpt, at most 20 hits, the total at the end ("100 or more" when the search reached its limit, also where the check left hits out, B-5); error codes exactly; archived spaces only with `archived` or named with `in` |
| `write` | A page, or the section a block opens (`#b3`), as Markdown with the markers read gave, against the version read (`base`); `dry_run` answers the check and the diff without saving |
| `replace` | One passage that stands once in the page, against `base`; `dry_run` as in `write` |
| `set_front_matter` | The title, the tags or the kind of a page |
| `create_page` | A page in a collection, folder or page, with its kind and text |
| `move` | An object into a collection, folder or page, or a block into another page |
| `trash` | An object with everything below it into the trash, where a person can restore it |
| `list_files` | The roots of the view Files, what lies in a folder, or the attachments of a page (module Files) |
| `read_file_page` | The text of a file page by page (`#page=2`) (module Files) |
| `upload_link` | A one-time link to upload a file into a folder, as a new version of a file, or to a page (module Files) |

Answers are compact (ADR-34): a list has at most 20 rows, an answer at most 100 KB. A page larger than that, or larger
than its share when several are read at once, comes as its outline with the address of each section. Errors are answers
the agent reads: "Not found" never tells whether something exists that the person may not see.

### The rules page

The conventions of an instance are one page in the wiki (ADR-33, ADR-34): the page an **administrator names** in the
browser, in the settings of a page of the kind `agent-notes` (*Make it the rules page for agents*, `PUT /api/rules-page`,
B-5). `context` names it to whoever may read it; the instructions of the server say to read it before writing. Agents
**only read** it: the one check refuses every other action of an agent on it and below it, whatever the entries say
(deciding entry `rules page`), and no agent sets or takes away the tag `rules` anywhere. People change it in the browser
like any page. Until B-5 it was the oldest page of that kind tagged `rules`, which an agent could take over by tagging an
older page; migration 038 kept such a page as the rules page.

```text
GET /api/rules-page    {"page": "<id>", "title": "…"}   the rules page the viewer reads, empty for none
PUT /api/rules-page    {"page": "<id>"}   administrators in the browser; "" for none
```

### Instructions

The server's instructions are short (well below the 4,096 characters clients take): the agent's limits, the addresses,
the audiences of the page kinds, the limits of answers and how to work. Everything else is on the rules page.

## Writing

A page is read as Markdown: front matter, then each block after a marker line `<!-- block b3 -->`, and the version of
the page, the newest change of its blocks. A write sends it back in the same form, with that version as `base`:

- A block with the marker of a block of the part written keeps its ID; its text, if changed, becomes a version.
- `<!-- block -->`, a marker without an ID, starts a new block; text before the first marker, or a text without any
  marker, is split before its headings into new blocks. An ID that is not in the part written is never taken: that
  block is made new.
- A block of the part written that is left out is deleted. Something another page embeds is not deleted: the answer
  names the pages, and a person decides in the browser what becomes of the uses, or the agent moves the block with
  `move`, which they follow.
- Front matter in a write is not applied; `set_front_matter` sets title, tags and kind.
- **Conflict:** a page changed since its version was read is refused, "page changed since you read it", with both
  versions. A write without `base` is refused too. Moving a block inside a page makes no version and is no conflict.
- **Dry run:** the whole write runs in a transaction that is then undone; the answer is the same, with nothing saved. The
  check finds the blocks the write would make, so a link to one of them is no `LINK` (B-5).
- **Unchanged is unchanged:** line ends become LF and blank lines at the start and end of a block go, so a page read and
  written back as it was, by a client that sends CRLF or adds blank lines, changes nothing (B-5).

Every answer has the diff of the blocks and the check of the page ([page-kinds.md](page-kinds.md)): errors block the
write, warnings come with their fix. For agents these matter most: `LINK` (a link to nowhere), `UNLINKED-FILE` (a file
nobody links to), `PAGE-SIZE` (a page above 25 KB), `EMBED`, `COPY` and the rules of the kind. The same check runs in the
browser.

What an agent writes is an agent's version (ADR-33): on human documentation the blocks it changed carry "written by AI,
not checked" until a person checks them, and what it took out is listed under the page. Every change is logged as a
change of the person, once an hour per object as in the browser (ADR-11); the versions keep each one.

## Structure

- `create_page` makes a page inside a collection, folder or page, with its text in one transaction: a text the check
  refuses makes no page (B-5). A space at the top is made by a person.
- `move` keeps the address of an object, so every link still leads there. A block moved to another page leaves a
  redirect: its old address leads to the new one, and what embedded it follows (ADR-22); `after` names a block of the
  page it goes to alone (`b3`). A redirect names its new place only to whoever may read it.
- **No publishing by an agent** (B-5): `move`, and copying, put nothing where someone may read it who may not read it
  now, a page, a folder or a block (a private draft into a shared collection, say); a person does that in the browser.
  Within the same readers, or to fewer, an agent moves freely.
- `trash` moves an object with everything below it to the trash, from where a person restores it within 30 days.
  Nothing is deleted for good by an agent, and no agent moves a whole space to the trash.

## Files

A file reaches an agent as text, page by page, never as its bytes: `read_file_page` gives one page of the text of a PDF,
the best text first (what the content says itself before what was recognised in pixels). The text of another file is
one page while it is short, and pages of about 20 KB, cut at a paragraph or a line, when it is longer (B-5); the search
names the same pages.

A file reaches UnderDoc through a **one-time upload link**, never inside a message:

1. `upload_link` starts an upload with the person's rights, for a name and an exact size, and answers a link
   `https://<host>/s/u/<token>` with a `curl` line: `curl -fsS -T <local-file> '<link>'`, the path of the local file as a
   placeholder and the link in single quotes, so nothing of the name runs in a shell (B-5). The link is a **bearer
   address**: whoever holds it sends the file, so it is passed on to nobody.
2. A program sends the file there as the body of one `PUT`, within 15 minutes. Its answer names what was made.

The link works once: taking it removes it, also when the upload fails. It is kept only as the hash of its token. It lies
below `/s/`, which the gate leaves open as it does for share links, since the program that sends the file is not signed
in; the token alone says what it may do. The rights are checked when the link is made and again when the file is made,
as an agent of the person. A request of a browser of another site is refused.

## Search

The search returns sections (ADR-24): one record per heading and what follows, one hit per object, codes in fields of
their own and exact, the rights as a filter and each hit checked once more in the database. The text of a PDF keeps its
pages apart, on the server and in the browser, so each page is records of its own and a hit names its page
(`#page=3`). A word takes a typing error from four letters on (T-24, measured in [search.md](search.md)).

## The archive

Archived spaces ([rights.md](rights.md#archived-spaces), T-34) are read only for everyone and left out of `context` and
`search` unless the agent asks with `archived: true` (the tree then lists them below the others, marked "archived, read
only"; without it, `context` says how many it left out). `read` and `used_in` open what lies inside by its address.
Every tool that changes something, and an `upload_link` made before, is refused there with "this space is archived".
Agents never archive or restore a space.

## What is not here yet

- **Gaps of the browser** (a tool that the browser cannot do is a missing feature of the interface, ADR-34): the
  description of a page has no way in the browser, so `set_front_matter` does not set it either; opening a hit of a PDF
  at its page (`#page=3`) shows the file from its start.
- The ways for something in use (delete everywhere, a copy for every use, a new master) are chosen by a person in the
  browser.
- Reading the text of an attachment of a page, history and older versions, folders, collections and groups are not
  tools: people do them in the browser (ADR-34).
- Moving a block inside a page makes no version, so it carries no hint of AI (T-20).
- Other sources (`source_search`, `source_read`) and the tools of connectors come with T-31.
- Automatic sections (T-30) do not exist yet, so `read` has nothing to count.
- **No rate limit** of its own on `/mcp`: the gate limits requests per token in front of UnderDoc (B-5); an operator
  without such a gate puts a limit into the proxy.
