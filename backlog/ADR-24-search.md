---
id: ADR-24
type: Decision
title: Search through a search service, Meilisearch by default and OpenSearch as a choice
status: erledigt
tags: [search, architecture]
created: 2026-11-16
---

## Context

Search is among the most important parts of UnderDoc (the maintainer, 2026-11-16). It must find error codes every time, rank by
relevance instead of age, and weigh a heading above a word somewhere in the text: searching for "card payment" usually means the
page or section about it. It covers pages, tables, lists, databases and files, including scanned ones. M-12 planned to measure
PostgreSQL full-text search first.

## Options

- **PostgreSQL full-text search** with trigrams: no extra service, but typo tolerance, ranking by field and German word forms
  have to be built by hand.
- **A search service:** Meilisearch (MIT), Typesense (GPL-3.0) or OpenSearch (Apache-2.0, needs a JVM).

## Decision

A search service, chosen by the maintainer on 2026-11-16, behind one interface of UnderDoc so that the engine can be chosen:

- **Meilisearch is the default.** **OpenSearch is offered as a second engine,** tested with the same suite and chosen in the
  configuration. Which one is better for German content is measured and documented.
- **Ranking:** title, then headings, then the text below them. Exact matches before typo matches. Newer only breaks ties.
- **Error codes and similar tokens** (`E-1042`, `0x80070005`, `ORA-00942`) go into a field of their own for exact matches as
  written and with their parts. A test corpus with such codes is a hard gate: every code must be the first hit.
- **Hits point to the block or section** they are in (ADR-22), open there and show the matching text highlighted.
- **Everything is searched:** pages, tables, native lists, cached views of databases and sources, and the text of files.
  Results only show what the reader may read, and a tenant only ever searches its own index (ADR-23). Passwords are never
  indexed.
- **Text of files** comes from three ways, each switched on or off by the administrator: in the **browser** of the person
  uploading (pdf.js for PDF text, Tesseract.js for OCR, language data served by UnderDoc), on the **server** in a queue with
  limits, and through an **AI model** for OCR of difficult scans, as paperlaiss does. The index notes where a text came from.
  Each way is set in layers: the hoster allows it, the tenant switches it on, and each user chooses within that, for example
  AI OCR for their own uploads only (2026-11-16). A tenant can set up one AI model for OCR that every user of the tenant may
  use, with a quota per user and for the tenant; a user may also bring a model of their own, which counts against no quota of
  the tenant.
- **Semantic search and an answer with sources** come on top, only when a model is set up (M-13), as hybrid search of the same
  service. Plain search comes first and has to be good on its own.
- **One search box with Ctrl+K** shows hits while typing, and a results page has filters by type, collection, author, date and
  tags, as in Confluence.

## Consequences

- A search service runs next to UnderDoc. Without it, UnderDoc still works and offers a simple search of titles from the
  database, and says so.
- The index is fed from the database through a queue, so a lost index can be rebuilt at any time.
- M-12 is updated: the measurement compares the two engines instead of deciding whether a service is needed at all.

## Note, 2026-11-17: measured

Both engines on a German corpus of 201 pages and 179 questions (docs/search.md, `internal/search/searchtest`):

| | Meilisearch v1.56.2 | OpenSearch 3.9.0 |
|---|---|---|
| codes, 104 questions: first hit | 100 % | 100 % |
| hit@1, all questions | 93 % | 94 % |
| hit@5, all questions | 98 % | 98 % |
| MRR, all questions | 0.951 | 0.964 |

- Codes are not searched as text at all: they go into fields of their own with their parts and are looked up exactly, in
  tiers (title, heading, anywhere, by a part), so no engine can match them through a typing error.
- Meilisearch stays the default: compounds are found best, little memory, nothing to tune. OpenSearch ranks German word
  forms and questions in sentences better.
- Settings: `UNDERDOC_SEARCH_ENGINE`, `UNDERDOC_SEARCH_URL`, `UNDERDOC_SEARCH_KEY_FILE`, `UNDERDOC_SEARCH_USER`,
  `UNDERDOC_SEARCH_INDEX`.
