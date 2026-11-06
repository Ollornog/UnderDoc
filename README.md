<p align="center"><img src="docs/logo.svg" width="250" height="250" alt="UnderDoc logo"></p>
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
- **Later:** proper search and optional AI over your own documents.

Everything starts read-only: you edit at the source, and the web editor comes later. See the
[milestones](backlog/README.md).

## Install

Not yet. The first installable version arrives with milestone [M-1](backlog/M-1-markdown-wiki.md).

## License

[MIT](LICENSE).
