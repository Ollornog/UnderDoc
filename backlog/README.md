# Backlog

<!-- GENERIERT von scripts/_backlog.py — nicht von Hand pflegen. Neu bauen: `python3 scripts/_backlog.py index` -->

Die Wahrheit sind die Einzeldateien in diesem Verzeichnis; diese Seite ist ihr Abzug.
Konventionen: [README-KONVENTION.md](README-KONVENTION.md).

## Meilensteine

* ☐ **[M-1](M-1-markdown-wiki.md)** Markdown wiki: a folder becomes a website — 2/2 erledigt
* ☐ **[M-2](M-2-files-and-excel.md)** Files and Excel: the share appears in the web — 0/3 erledigt
* ☐ **[M-3](M-3-single-source.md)** Single source: values, snippets, embedding — 0/1 erledigt
* ☐ **[M-4](M-4-sources.md)** External sources with cache — noch keine Aufgaben
* ☐ **[M-5](M-5-permissions-sharing.md)** Permissions and sharing — noch keine Aufgaben
* ☐ **[M-6](M-6-lists.md)** Native lists — noch keine Aufgaben
* ☐ **[M-7](M-7-output-access.md)** Output and more file access — noch keine Aufgaben
* ☐ **[M-8](M-8-editor.md)** Web editor — noch keine Aufgaben
* ☐ **[M-9](M-9-inline-editing.md)** Inline editing — noch keine Aufgaben
* ☐ **[M-10](M-10-realtime.md)** Working together — noch keine Aufgaben
* ☐ **[M-11](M-11-drawing.md)** Drawing — noch keine Aufgaben
* ☐ **[M-12](M-12-search.md)** Proper search — noch keine Aufgaben
* ☐ **[M-13](M-13-ai.md)** AI integration — noch keine Aufgaben
* ☐ **[M-14](M-14-workspace.md)** Workspace: three columns, views, hierarchy, settings and metadata — 0/2 erledigt
* ☐ **[M-15](M-15-windows.md)** Windows and tabs: documents side by side — 0/1 erledigt
* ☐ **[M-16](M-16-favorites-mentions-inbox.md)** Favorites, mentions and inbox — noch keine Aufgaben
* ☐ **[M-17](M-17-import-export.md)** Import, export and save as — noch keine Aufgaben
* ☐ **[M-18](M-18-calendar.md)** Calendar — noch keine Aufgaben
* ☐ **[M-19](M-19-forms.md)** Forms and simple surveys — noch keine Aufgaben
* ☐ **[M-20](M-20-review-and-git.md)** Reviewed documents and Git — 0/1 erledigt
* ☐ **[M-21](M-21-websites.md)** Websites: build and publish — 0/1 erledigt
* ☐ **[M-22](M-22-mail.md)** Mail in the workspace — noch keine Aufgaben
* ☐ **[M-23](M-23-network-module.md)** IP addresses and devices — noch keine Aufgaben
* ☐ **[M-24](M-24-private-area.md)** Private area — noch keine Aufgaben
* ☐ **[M-25](M-25-bookmarks.md)** Bookmarks — noch keine Aufgaben
* ☐ **[M-26](M-26-news.md)** News from subscribed feeds — noch keine Aufgaben
* ☐ **[M-27](M-27-file-sources.md)** Further file sources — noch keine Aufgaben
* ☑ **[M-28](M-28-first-build.md)** First build: sign-in, account, collections and documents as objects — noch keine Aufgaben
* ☐ **[M-29](M-29-recipes.md)** Recipes — noch keine Aufgaben

## Aufgaben

* ☐ **[T-1](T-1-measure-office-saving.md)** Measure how Excel and LibreOffice save over Samba · M-2
* ☑ **[T-2](T-2-go-ui-foundation.md)** Web foundation and design adapter for a Go app · M-1
* ☐ **[T-3](T-3-mermaid-under-strict-csp.md)** Render Mermaid under a strict CSP · M-3
* ☐ **[T-4](T-4-versions-and-audit.md)** Store versions and record who changed what · M-2
* ☐ **[T-5](T-5-workspace-mockups.md)** Build workspace mockups in several variants and choose · M-14
* ☐ **[T-6](T-6-window-loading.md)** Choose how windows load their documents · M-15
* ☐ **[T-7](T-7-review-model.md)** Decide how reviews work, with which forge · M-20
* ☐ **[T-8](T-8-website-kinds.md)** Settle what the site builder covers · M-21
* ☐ **[T-9](T-9-which-tree-decides-roles.md)** Decide which tree decides inherited roles · M-2
* ☐ **[T-10](T-10-phone-layout-mockups.md)** Mock up the phone layout · M-14

## Fehler

* ☑ **[B-1](B-1-folder-links-broken.md)** Links to folders count as broken · M-1

## Entscheidungen (ADR)

* ☑ **[ADR-1](ADR-1-build-instead-of-fork.md)** Build our own instead of forking a wiki
* ☑ **[ADR-2](ADR-2-go-and-postgres.md)** Go and PostgreSQL
* ✗ **[ADR-3](ADR-3-files-as-truth.md)** Files in folders are the source of truth · abgelöst durch ADR-13
* ☑ **[ADR-4](ADR-4-read-only-first.md)** Read-only first
* ☑ **[ADR-5](ADR-5-one-instance-per-customer.md)** One instance per customer
* ☑ **[ADR-6](ADR-6-mit-license.md)** MIT license
* ☑ **[ADR-7](ADR-7-passwords-outside-markdown.md)** Passwords never enter Markdown
* ☑ **[ADR-8](ADR-8-one-source-per-list.md)** Every list has exactly one source
* ☑ **[ADR-9](ADR-9-sign-in-gate.md)** TinySesam as the sign-in gate
* ☑ **[ADR-10](ADR-10-file-access-doors.md)** Ways to reach the files: SMB, WebDAV, browser
* ☑ **[ADR-11](ADR-11-versions-and-audit.md)** Versions and audit are built in, Seafile is not the base
* ☑ **[ADR-12](ADR-12-markdown-first.md)** Start with Markdown only
* ☑ **[ADR-13](ADR-13-documents-and-folders.md)** Documents are objects, the folder tree is separate
* ☑ **[ADR-14](ADR-14-one-topic-at-a-time.md)** One topic at a time, built as modules
* ☑ **[ADR-15](ADR-15-workspace-of-typed-documents.md)** A workspace of typed documents, with three views
* ☑ **[ADR-16](ADR-16-how-the-workspace-works.md)** How the workspace looks and works
* ☑ **[ADR-17](ADR-17-guiding-principles.md)** Guiding principles for every feature
* ☑ **[ADR-18](ADR-18-object-ids-and-addresses.md)** Object IDs and addresses
* ☑ **[ADR-19](ADR-19-pages-in-the-database.md)** Pages live in the database, as blocks with IDs
* ☑ **[ADR-20](ADR-20-who-sees-what-before-roles.md)** Who sees and changes what, before roles arrive
