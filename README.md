<h1 align="center">UnderDoc</h1>

<p align="center"><b>English</b> · <a href="i18n/README.de.md">Deutsch</a></p>

**Your team's knowledge in one place, without taking Excel away from anyone.**

> **Status: concept.** There is no usable program yet. The plan is in [`docs/concept.md`](docs/concept.md)
> and [`backlog/`](backlog/).

## Why

Shared documentation tends to end up in Excel files nobody can find, in a wiki nobody updates, or in
both at once with different numbers. UnderDoc is a self-hosted wiki built around one rule: **every
fact has exactly one source.**

## What it will do

- **Plain Markdown pages** in normal folders, shown as a wiki, as private notes and as a file tree.
- **Excel stays Excel.** Files live on a Windows share and open in Excel or LibreOffice as usual. The web
  shows the same file.
- **Embed instead of copy.** Tables, database views, images, snippets and diagrams come from their source.
- **Values as variables.** Write `{{tower.height}}` on ten pages and change it once.
- **Native lists** with links between them, for customers, contacts and the like.
- **External sources** with caching, plus exports, scheduled reports and an API for other systems.
- **Mermaid diagrams**, also fed by values and snippets.
- **Company permissions:** users, groups, OIDC and SAML, plus public sharing.

Everything starts read-only: you edit at the source, and the web editor comes later. See the
[milestones](backlog/README.md).

## Install

Not yet. The first installable version arrives with milestone [M-1](backlog/M-1-read-core.md).

## License

[MIT](LICENSE).
