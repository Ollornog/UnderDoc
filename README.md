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
- **Company permissions:** users, groups, OIDC and SAML, plus public sharing.
- **Later:** documents side by side, an inbox, a calendar, forms, reviews through Git, a site builder, proper search and
  optional AI over your own documents.

Everything starts read-only: you edit at the source, and the web editor comes later. See the
[milestones](backlog/README.md).

## Install

There are no releases yet. Every state on `main` is published as a container image for amd64 and arm64,
`ghcr.io/ollornog/underdoc:main-<first 12 hex of the commit>`; pin it by tag and digest. Or build it from source with Go
or Docker, as shown below.

## Try it

UnderDoc has no sign-in of its own, so it binds to loopback by default. A sign-in gate in front of it checks who someone
is and passes that on in the headers `Remote-User`, `Remote-Id`, `Remote-Name`, `Remote-Email` and `Remote-Groups`, which
the gate must remove from every incoming request first. Without `Remote-User`, UnderDoc shows only a page that says so.
How people sign in is up to the gate: [TinySesam](https://github.com/Ollornog/TinySesam) offers OIDC (for example
PocketID or Microsoft Entra ID), SAML, LDAP, TOTP and passkeys, and any other gate that sets these headers works too.
To try it on your own machine, let a made-up user stand in for the gate:

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
| `UNDERDOC_ADMIN_GROUP` | Group of the gate whose members see the administration | `underdoc-admins` |
| `UNDERDOC_DEV_USER` | A login that stands in for the gate. Never behind a real gate | off |
| `UNDERDOC_DATABASE_URL` | PostgreSQL for accounts and what is made in the web, for example `postgres://underdoc@db/underdoc` | none: the folder is all there is |

**Export and import** need the database. `underdoc export <dir>` writes every collection as normal files: directories,
Markdown files with front matter, and a hidden comment before each block that keeps its ID. `underdoc import --owner <login>
<dir> <title>` reads such a tree, or any folder of Markdown files, into a new collection. The layout and the rules are in
[docs/export.md](docs/export.md).

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
