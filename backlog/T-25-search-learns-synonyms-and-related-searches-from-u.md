---
type: Task
title: Search learns synonyms and related searches from use
status: offen
sprint: S2
points: 8
id: T-25
milestone: M-12
created: '2026-11-23'
---
# T-25 — Search learns synonyms and related searches from use

Search gets better by itself, as the maintainer asked on 2026-11-23.

- **Reformulations, no model needed:** someone searches "Kasse", opens nothing and right after searches "Kassa" and opens a hit. When several people do the same, the pair becomes a proposed synonym, loaded into the engine's synonyms.
- **Searched together:** terms often searched in one session show as "others also searched".
- **Searches without hits** form a list for administrators: missing documentation or a missing synonym. With an AI model set up (M-13), it proposes synonyms, abbreviations and spellings for them.
- **At write time:** an agent that writes a page adds common spellings and abbreviations as search terms (ADR-33, ADR-34).
- Learned synonyms take effect at once with the hint "learned, not checked". An administrator confirms or discards them, as with AI changes in ADR-33.

Rights and privacy come first:

- A suggestion shows only if it leads to hits the searching person may read, so it never reveals the name of something hidden.
- Only what several different people searched is learned; a single search never shows to others.
- Search logs are personal data: stored without names, within the tenant, deleted after a fixed time, and the learning can be switched off.
- Codes stay exact (ADR-24).
