---
id: M-13
type: Milestone
title: AI integration
status: offen
tags: [ai]
created: 2026-11-06
---

Questions over the company's own documents. Optional and off by default.

**Done when this is true:**

- The model is configurable, including self-hosted ones, and nothing leaves the server without explicit configuration.
- Answers only draw on what the asking user may read, and they name their sources.
- An AI drawer at the right edge, opened from the top bar, knows the document in the active window. It can summarize it, find related
  documents and draft a note from it (ADR-16).
- Search gains the strategy similar meaning, and an AI answer with sources on top of the hits (ADR-16). Without a set-up AI, neither appears.
- Passwords never reach a model, and public shares offer no AI.
- Other AI tools can read through the same permissions, for example over MCP.
- Every use is logged.
