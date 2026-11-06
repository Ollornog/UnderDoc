# Backlog

<!-- GENERIERT von scripts/_backlog.py — nicht von Hand pflegen. Neu bauen: `python3 scripts/_backlog.py index` -->

Die Wahrheit sind die Einzeldateien in diesem Verzeichnis; diese Seite ist ihr Abzug.
Konventionen: [README-KONVENTION.md](README-KONVENTION.md).

## Meilensteine

* ☐ **[M-1](M-1-markdown-wiki.md)** Markdown wiki: a folder becomes a website — 1/1 erledigt
* ☐ **[M-2](M-2-files-and-excel.md)** Files and Excel: the share appears in the web — 0/2 erledigt
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

## Aufgaben

* ☐ **[T-1](T-1-measure-office-saving.md)** Measure how Excel and LibreOffice save over Samba · M-2
* ☑ **[T-2](T-2-go-ui-foundation.md)** Web foundation and design adapter for a Go app · M-1
* ☐ **[T-3](T-3-mermaid-under-strict-csp.md)** Render Mermaid under a strict CSP · M-3
* ☐ **[T-4](T-4-versions-and-audit.md)** Store versions and record who changed what · M-2

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
