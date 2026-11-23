<p align="center"><img src="docs/logo.svg" width="250" height="250" alt="UnderDoc logo"></p>
<h1 align="center">UnderDoc</h1>

<p align="center"><b>English</b> · <a href="i18n/README.de.md">Deutsch</a></p>

**Your team's knowledge in one place, without taking Excel away from anyone.**

> **Status: early.** A folder of Markdown files already works as a read-only wiki (M-1). Everything below that is not part
> of it is still a plan, see [`docs/concept.md`](docs/concept.md) and [`backlog/`](backlog/).

## Why

Shared documentation tends to end up in Excel files nobody can find, in a wiki nobody updates, or in
both at once with different numbers. UnderDoc is a self-hosted wiki built around one rule: **every
fact has exactly one source.**

## What it will do

- **Plain Markdown pages** arranged in folders, shown as a wiki, as private notes and as a file tree.
- **Documents with owners,** editors, commenters and viewers. Moving one changes where it appears, not what it is.
- **Excel stays Excel.** Files live on a Windows share and open in Excel or LibreOffice as usual. The web
  shows the same file.
- **Embed instead of copy.** Tables, database views, images, snippets and diagrams come from their source.
- **Values as variables.** Write `{{tower.height}}` on ten pages and change it once.
- **Native lists** with links between them, for customers, contacts and the like.
- **External sources** with caching, plus exports, scheduled reports and an API for other systems.
- **Mermaid diagrams**, also fed by values and snippets.
- **Company permissions:** users, groups and roles from the sign-in gate (which speaks OIDC, SAML and LDAP), plus public sharing.
- **Later:** documents side by side, an inbox, a calendar, forms, reviews through Git, a site builder, proper search and
  optional AI over your own documents.

Everything starts read-only: you edit at the source, and the web editor comes later. See the
[milestones](backlog/README.md).

## Install

There are no releases yet. Every state on `main` is published as a container image for amd64 and arm64,
`ghcr.io/ollornog/underdoc:main-<first 12 hex of the commit>`; pin it by tag and digest. Or build it from source with Go
or Docker, as shown below.

## Try it

UnderDoc binds to loopback by default and has no sign-in of its own: people sign in only through the gate in front of
it, [TinySesam](https://github.com/Ollornog/TinySesam) ([ADR-32](backlog/ADR-32-sign-in-only-through-the-gate.md),
[docs/sign-in.md](docs/sign-in.md)). The gate checks who someone is and passes that on in the headers `Remote-User`,
`Remote-Id`, `Remote-Name`, `Remote-Email`, `Remote-Groups`, `Remote-Auth` and `Remote-Kind`, which its proxy removes
from every incoming request first, or in its signed gate token; TinySesam's Go client checks either. A person is bound
by `Remote-Id`, administrators hold the role of the app (`underdoc-admin`), spaces at the top are made by them and the
holders of `underdoc-spaces`, a request without a known `Remote-Kind` is refused, a service account only comes in with
its ID allowed, and a request whose `Remote-Auth` is not `session` counts as an agent. In an emergency `underdoc admin grant <remote-id>` marks an administrator. To try it on
your own machine, let a made-up user stand in for the gate:

```bash
UNDERDOC_DEV_USER=me go run ./cmd/underdoc serve ~/notes     # http://127.0.0.1:8080

docker build -t underdoc .
docker run --rm --read-only --cap-drop ALL --security-opt no-new-privileges \
  -p 127.0.0.1:8080:8080 -v ~/notes:/data:ro underdoc
```

| Setting | Meaning | Default |
|---|---|---|
| `UNDERDOC_DIR` | The folder, or the argument of `serve` | none |
| `UNDERDOC_ADDR` | Address of the wiki | `127.0.0.1:8080`, in the container `:8080` |
| `UNDERDOC_HEALTH_ADDR` | Address of `GET /healthz` | `127.0.0.1:8081` |
| `UNDERDOC_SCAN_INTERVAL` | How often the folder is checked | `2s` |
| `UNDERDOC_DEV_UI` | `1` shows all components at `/dev/ui` | off |
| `UNDERDOC_ADMIN_ROLE` | The role of the app in the gate whose holders administer UnderDoc ([docs/sign-in.md](docs/sign-in.md#the-gate)) | `underdoc-admin` |
| `UNDERDOC_SPACES_ROLE` | The role of the app in the gate whose holders may create spaces, the collections at the top ([docs/rights.md](docs/rights.md#spaces)) | `underdoc-spaces` |
| `UNDERDOC_SERVICE_ACCOUNTS` | The lasting IDs (`Remote-Id`) of the service accounts allowed in; any other is refused ([docs/rights.md](docs/rights.md#service-accounts)) | none |
| `UNDERDOC_GATE_MODE`, `UNDERDOC_GATE_PROXIES`, `UNDERDOC_GATE_*` | How the sign-in gate reaches UnderDoc: its headers from the addresses of its proxy, or its token; the gate's Go client checks either ([docs/sign-in.md](docs/sign-in.md#the-gate)) | header mode, any address |
| `UNDERDOC_DEV_USER` | A login that stands in for the gate. Never behind a real gate | off |
| `UNDERDOC_DATABASE_URL` | PostgreSQL for accounts and what is made in the web, for example `postgres://underdoc@db/underdoc` | none: the folder is all there is |
| `UNDERDOC_TENANTS` | `one` tenant; `many` waits for the gate to name the tenant and does not start yet ([docs/tenants.md](docs/tenants.md)) | `one` |
| `UNDERDOC_SECRET_KEY` | 32 random bytes in base64 that seal secrets (passwords in the text, app passwords of Samba, secrets of other systems); without it, these are not offered | none |
| `UNDERDOC_SECRET_KEY_OLD` | The key before a rotation, until `underdoc rekey` sealed everything again ([docs/secrets.md](docs/secrets.md)) | none |
| `UNDERDOC_MODULES` | Modules allowed, each on by default or with `:off` off by default ([docs/modules.md](docs/modules.md)) | every module |
| `UNDERDOC_STORE_DIR` | The content store of the files, with the database; limits and versions in [docs/files.md](docs/files.md) | none: no files |
| `UNDERDOC_SEARCH_URL` | The search service, Meilisearch by default or OpenSearch (`UNDERDOC_SEARCH_ENGINE`), with its key in `UNDERDOC_SEARCH_KEY_FILE` or `UNDERDOC_SEARCH_KEY` | none: titles only |

**Search** (ADR-24) goes through a search service, Meilisearch by default, OpenSearch as a choice: error codes are found exactly as written and
first, then titles before headings before text, German compounds and typing errors included, and a hit opens at its
section. Ctrl+K searches from anywhere. Without a search service only titles are searched. Settings, ranking and the
measurement on a German test corpus are in [docs/search.md](docs/search.md). The services next to UnderDoc (Meilisearch,
OpenSearch, Mailpit, Gotenberg) start from [`deploy/compose.yml`](deploy/compose.yml), see [docs/services.md](docs/services.md).
A new Meilisearch image takes over the database of the old one by itself (`MEILI_UPGRADE_DB`); that is not atomic, so
take a snapshot of the volume first ([Updating Meilisearch](docs/services.md#updating-meilisearch)).

**Export and import** need the database and work on one tenant, Main unless `--tenant <id>` names another.
`underdoc export <dir>` writes every collection as normal files: directories,
Markdown files with front matter, and a hidden comment before each block that keeps its ID. `underdoc import --owner <login>
<dir> <title>` reads such a tree, or any folder of Markdown files, into a new collection. The layout and the rules are in
[docs/export.md](docs/export.md).

**Files** need the database and `UNDERDOC_STORE_DIR`: the view Files has the folder of the company, a folder of each user
and collections, with uploads that resume, versions, quotas and a trash. Every version lies once in a content store named
by its hash, and an export takes the files along. How it works: [docs/files.md](docs/files.md). Spreadsheets (`.xlsx`,
`.xlsm`, `.ods`, `.csv`) show as tables, read by UnderDoc itself: [docs/tables.md](docs/tables.md).

**Samba** shows the folder of the company, each user's own folder and shared collections as network drives
(`UNDERDOC_SAMBA`): a companion container runs smbd, UnderDoc steers it over a Unix socket without root, each account
makes an app password in its profile, and saving in Office or LibreOffice becomes a version. How it works:
[docs/samba.md](docs/samba.md).

**Rights** hold for documents and files alike: an owner, and the actions read, comment, create, write, delete, share and
manage, each inherit, allow or deny, for persons, groups (local and nested, or taken over from the gate) and the
whole tenant, with the roles read, comment, edit, members (manage without delete) and manage as presets. The nearest entry wins, deny beats allow on the
same level, and the built-in group Agents (every sign-in that is not a session, and every service account) can be denied
as a ceiling. Groups are archived, never deleted, and a space goes into the archive without a limit: hidden from the tree
and the search until asked for, read only through every door, restored by its owner or an administrator in the browser.
Deleting for good from the trash is for the owner or an administrator in the browser, with the name typed in. Private notes stay private, also from administrators. How it works: [docs/rights.md](docs/rights.md).

**Passwords in the text** are references, `::secret{id="…"}`: the value lives sealed in the database, shows as a mask
with Reveal and Copy, comes from the server on a click only, and every reveal is logged. Whoever reads the page reveals
it, or fewer if its managers say so; it never reaches the search, an export, a share link or the share. Operations
documentation, not a vault: [docs/secrets.md](docs/secrets.md).

**Page kinds** say who a page is for: structure and records, agent documentation and reference, or human documentation
(how-to, explanation, tutorial). Every write is checked against the rules of its kind; errors block, warnings come back
with how to fix them, such as "embed instead of copying". Agents read the raw page with its front matter. What an agent
changes in human documentation says "written by AI, not checked" until a person checks the diff or takes it back:
[docs/page-kinds.md](docs/page-kinds.md).

**Agents over MCP** reach the pages at `/mcp` (ADR-34): signed in through the gate with OAuth, always as an agent of
their person, so a space with "no agents" stays closed and rights, share links, deleting for good and passwords stay out
of reach. A small core of tools: `context` to start, `read` for pages and sections with the version to write against,
`used_in`, one `search` over sections of pages and pages of PDFs, `write` and `replace` against that version with a dry
run, the diff and the check in every answer, front matter, a new page, `move`, `trash`, and files read page by page and
uploaded through a one-time link. The conventions are a rules page in the wiki: [docs/mcp.md](docs/mcp.md).

**The web editor** writes a page of the database in one editor with a live preview: Markdown that looks rendered away
from the cursor, a bar above the page, `/` for the kinds of blocks, and saving as you type. It is built with Node from
`web/editor` by `scripts/build-js.sh` before `go build` (the built bundle is not in the repository). How it works and how to
build it: [docs/editor.md](docs/editor.md).

**Writing pages:**

- A page is a `.md` file, written in any text editor. Hidden files and symbolic links are skipped.
- Give a page a stable address with front matter, so it survives a move:

  ```markdown
  ---
  id: network-plan
  title: Network plan
  description: Router, switches and the radio link.
  tags: [network, main-site]
  ---
  ```

- Link with `[[network-plan]]`, `[[Network plan]]`, `[[network-plan|some text]]` or `[[network-plan#Heading]]`. A link by ID or
  title keeps working when a file moves. Normal relative links work too, but they depend on the path.
- Images next to a page work as usual, with `![Map](map.png)`. Images from other hosts become links, as pages load nothing from third parties.
- `index.md` or `README.md` in the top folder is the start page.
- Anything broken shows on the page itself and in the list under **Problems**.

## License

[MIT](LICENSE).
