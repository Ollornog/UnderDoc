# Changelog

All notable changes to this project are documented in this file. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project uses
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

- `scripts/vendor-c22.sh` takes the licence text of the Inter font from C22 instead of keeping its own copy current by hand.

### Added

- Project skeleton: concept, backlog with milestones and decisions, CI, repository hygiene.
- Plan for versions and audit (ADR-11).
- Logo and favicon.
- Plans for search (M-12) and AI (M-13).
- Plans for the workspace with tabs and windows, favorites and inbox, import and export, calendar, forms, reviews through Git
  and a site builder (M-14 to M-21), and an order of work: one topic at a time, built as modules (ADR-14).
- Plan for a workspace of typed documents with three views: workspace, files and resources (ADR-15).
- How the workspace looks and works: icon column, favorite collections, tabs, windows that come and go, a gear per document, tags and
  search strategies (ADR-16).
- Plans for more document types (to-do list, kanban board, video, PowerPoint, quick notes), settings with modules an administrator
  switches on and off, mail (M-22), IP addresses and devices (M-23), and a private area (M-24).

### Added

- A folder of Markdown files appears as a wiki: tree, pages, `[[wiki links]]`, backlinks, table of contents, images next to a
  page, full-text search, recent changes and a list of everything broken (M-1).
- Pages with an `id:` in their front matter keep their address and their links when the file moves.
- Changes on disk appear within seconds, without a restart.
- One binary or one container, read-only access to the folder, a strict CSP and a health endpoint.

### Fixed

- A link to a folder opens that folder instead of counting as broken (B-1).

### Changed

- Documents have their own identity and roles, separate from the folder tree (ADR-13, replaces ADR-3).
- The first milestone is now a Markdown wiki only. Excel, Samba and versions moved to M-2 (ADR-12).
