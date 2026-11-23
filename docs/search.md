# Search

UnderDoc searches through a search service (ADR-24): **Meilisearch** by default, **OpenSearch** as a choice. Without
one, it still works: it searches titles and lists what carries the chosen tags, and says so. This page describes how the search finds and ranks, how to
set it up, and what was measured.

## Setting it up

Search needs the database (`UNDERDOC_DATABASE_URL`): the queue that feeds the index lives there. Without the database the
folder of M-1 is searched in memory, as before.

| Setting | Meaning | Default |
|---|---|---|
| `UNDERDOC_SEARCH_ENGINE` | `meilisearch`, `opensearch` or `none` | `meilisearch` when a URL is set, else `none` |
| `UNDERDOC_SEARCH_URL` | Address of the search service, for example `http://127.0.0.1:7700` or `https://127.0.0.1:9200` | none: titles only |
| `UNDERDOC_SEARCH_KEY_FILE` | A file that holds the API key of Meilisearch, or the password of OpenSearch | no key |
| `UNDERDOC_SEARCH_KEY` | The same key from the environment, for a container that gets it from a `.env` file; never both | no key |
| `UNDERDOC_SEARCH_USER` | The user of OpenSearch for basic authentication | none: no authentication |
| `UNDERDOC_SEARCH_INDEX` | What every index name starts with, so installations can share one service: lower case letters, digits, dashes | `underdoc` |

OpenSearch needs a user with the rights to create, delete, write and search the indexes `<prefix>-*`; its TLS
certificate must be trusted by the system. OpenSearch cannot change the analysis of an index in place, so a new version of
UnderDoc's mapping drops the index and fills it again from the queue.

The key of Meilisearch needs the rights to manage indexes and documents, to search, and to read tasks; the master key
works, a key with only the actions `indexes.*`, `settings.*`, `documents.*`, `search` and `tasks.get` on the indexes
`<prefix>-*` is better. The key is read once at start, from the file or the environment, and never reaches a log. A file
is the better place where the installation can give one: a variable of the environment is visible to whoever may inspect
the container.

UnderDoc keeps one index per tenant (ADR-23), named `<prefix>-<tenant>`; with one tenant that is `underdoc-main`. Each
tenant has its own queue in the database, kept apart by row-level security like every table, and its own indexer, also
for a tenant made later (looked for every minute). The pages of the folder belong to Main only.
Nobody but UnderDoc talks to the search service: the browser asks UnderDoc, and UnderDoc asks the service with the
readers and filters of the person asking. The service should therefore listen only where UnderDoc reaches it.

`underdoc reindex [--tenant <id>]` queues every object of a tenant again (Main without `--tenant`), for example after a restore of the search service. A new index, or a new
version of its settings, is filled from the queue by itself when `serve` starts.

The search service is not part of the health of UnderDoc (`/healthz`): when it is down, the search falls back to titles,
the results page says so, and the queue keeps every change until the service answers again.

## What is found

Everything a reader may read, and nothing else:

- **Pages, folders and collections** of the database, the cards of the quick notes (by their owner only), and the pages
  of the folder (M-1). Tables in a page are found by
  their cells.
- **Files** of the view Files (M-2, ADR-28) by their name and the text of their current version, and the **attachments
  of a page**, such as a pasted scan, by their text: a hit on one opens its page at the block that shows it. Of the texts
  a file has (docs/files.md, "Text for the search"), the one its content gives (server, then browser) comes before what
  was recognised in pixels (a model, then OCR on the server); each record says where its text came from (`origin`, type
  `file`). Files follow the rule of the documents: whoever holds a role on the file reads it (ADR-29,
  [rights.md](rights.md)), and the folder of a user is read by that user only. The folders and roots of the view Files are places, not content, and are not found themselves.
- **Spreadsheets** (`.xlsx`, `.xlsm`, `.ods`, `.csv`, ADR-31, [tables.md](tables.md#search)) are found by the cells of
  their visible sheets instead of the text the server reads: a record for each run of rows of a sheet, headed by the
  sheet, and a hit opens the table view at `<sheet>!<range>`. Hidden sheets, rows and columns are not found.
- A new version, a text read later and an attachment added or removed put the file or page into the queue by themselves
  (triggers of the database, migration 020), whichever way the text came.
- Later sources only register with the indexer (`search.Source`): tables and lists (M-6) and cached views of sources
  (M-4). A source whose records find more than before raises its version (`Version()`), and the index of every tenant is
  filled again once, as for new settings of the engine.
- **Passwords never**: the directives of ADR-7 (`::secret{…}`) and their IDs are cut out before anything is indexed.

**Rights** are a filter in the index, and a check afterwards: every record keeps its owner, who always finds it, and the
levels of entries on read from it up, each with the tokens allowed and denied there (`all`, `account:<id>`,
`group:<id>`, `direct:<id>`, fields `readers`, `l0a` … `l5d`, ADR-36), and whether no agent may find it (`no_agents`).
Every query carries the tokens of the person asking, of their groups at any depth and of the groups they are in
directly, and whether they are an agent; the first level that names one of their tokens decides, deny before allow
([rights.md](rights.md#one-check)). A change of entries queues the object and everything below it, and each hit is
checked once more in the database before it shows. The counts beside the hits (type, collection, author, tags) are made
from the checked hits, not taken from the engine: the tokens of a reader leave out those of groups whose allow does not
hold for them (archived groups; for a service account everyone and the roles of the gate), so their denies cannot hold
in the index, and its counts could tell of what the reader may not find (B-4). A query without readers is refused. The tenant is separated by its
own index.

**Sections:** a page is indexed in sections, one per heading and what follows it up to the next heading (ADR-22). A hit
names its section: it opens at that block or heading, with the matching words marked there. Each page shows once in the
results, with its best section. Very long sections are split, so every word stays findable.

## How it ranks

1. **Error codes first, exactly as written.** `E-1042`, `0x80070005`, `ORA-00942`, `HTTP 503`, `KB5034441`,
   `SQLSTATE 40P01`, `Event ID 4625`, `CVE-2024-3094`, `E_ACCESSDENIED`, `max_connections`, `Druckerfehler 79`:
   anything with a digit, an identifier with an underscore, or a word such as HTTP, Port, Fehler or a word in capitals
   followed by a number. Codes go into fields of their own in a canonical form (lower case, no separators), with their
   parts (`ORA-00942` also gives `00942` and `942`, `0x80070005` gives `80070005`, `KB5034441` gives `5034441`). A query
   with a code looks it up there, in this order: in a title (a title that starts with the code first), in a heading, as
   a whole anywhere, then by a part. **A code is never searched as text**, so it never matches through a typing error:
   `0x80070015` does not find `0x80070005`, and a code that is nowhere finds nothing.
2. **Then the full text** of the words of the query: title before headings before tags before text, exact words before
   words with a typing error, the words close together before far apart, and newer only to break a tie.
3. **German** (Meilisearch; OpenSearch see below): compounds are split (`Warteschlange` finds `Druckerwarteschlange`, `zahlung` finds `Kartenzahlung`), the
   last word of a query is also a prefix (`Etikett` finds `Etiketten`), and typing errors are forgiven in words of five
   letters and more (one) and nine and more (two), never in numbers. Common words such as `der`, `nicht` or `wie` are
   ignored in a query.

The strategy **exact words** on the results page finds the words only as written and in their order, without typing
errors. **Similar meaning** comes with AI (M-13) as a hybrid search of the same service; until then it is the full text.

## In the workspace

- **Ctrl+K** (Cmd+K) opens the quick search anywhere but in the editor, where Ctrl+K makes a link. It shows pages and
  folders by their title at once, and the hits of the full text as you type, each with its place, section and a snippet
  with the matches marked. The arrows choose, Enter opens, Escape closes; the last entry opens all results.
- **/** puts the cursor into the search field at the top; typing there shows the results page with its filters, as in
  Confluence: type, collection, author, the time of the last change, and tags that all must be there. Enter opens the
  first hit.
- A hit opens its page at the section, scrolled there, with the matching words marked.

## Tests

The search is tested against real engines in throwaway containers, started by `scripts/search-engines.sh` with names and
ports of their own, so parallel runs never meet. `scripts/check.sh` starts them and fails without Docker: a skipped search
test is not a green one. Engines that run already can be named with `UNDERDOC_TEST_MEILISEARCH_URL` and `UNDERDOC_TEST_OPENSEARCH_URL`. The images are
pinned by tag and digest in `scripts/search/Dockerfile.*`, which only carry the pin, and Dependabot moves them. The CI
runs the same images as `services:` of its job and names them this way; `tests/test_repo.py` holds the services and
`deploy/compose.yml` to the pins. Dependabot moves only the Dockerfiles (a workflow cannot read its services from a
file, and the Compose file is left to the pins), so a pin it moved turns the check red until `.github/workflows/ci.yml`
and `deploy/compose.yml` follow in the same pull request: copy the `FROM` line. `scripts/check-services.sh` then proves
that the new Meilisearch takes over a database of the version before ([services.md](services.md#updating-meilisearch)).

The OpenSearch of the tests runs with `cluster.routing.allocation.disk.threshold_enabled=false`: its disk watermarks
look at the disk of the host, and above the high watermark (90 %) OpenSearch refuses to create an index, so a test would
turn red because the machine is full. `deploy/compose.yml` keeps the watermarks: in production they protect the data.

Every engine passes the same functional tests (`internal/search/searchtest`): readers, deny by levels, tenants, replacing and removing,
filters, one hit per page, codes exactly and never through a typing error, typing errors in words, title before heading
before text, marks, exact words.

### The German corpus

`internal/search/searchtest/corpus` holds 201 German pages of a made-up retail company: an error catalogue of 64 codes in
many formats, 40 runbooks, 37 chapters of a handbook, 30 sets of minutes and 30 pages of IT documentation, with 179
questions and the page, and for some the section, that should come first. The questions of the kinds `code`,
`code-variant` (the code written otherwise, or a part of it) and `code-absent` (a code that is nowhere) are a **hard
gate**: each one has to be the first hit, or find nothing at all. For the other kinds, a floor below the measured value
keeps the ranking from getting worse unnoticed.

`UNDERDOC_SEARCH_REPORT=<file>` appends the tables of a run to a file.

#### Meilisearch v1.56.2 (2026-11-17)

| meilisearch | questions | hit@1 | hit@5 | MRR | section |
|---|---|---|---|---|---|
| code | 65 | 100 % | 100 % | 1.000 | – |
| code-variant | 32 | 100 % | 100 % | 1.000 | – |
| code-absent | 7 | 100 % | 100 % | 1.000 | – |
| title | 18 | 100 % | 100 % | 1.000 | – |
| heading | 15 | 100 % | 100 % | 1.000 | 15/15 |
| wordform | 10 | 70 % | 100 % | 0.808 | – |
| compound | 10 | 100 % | 100 % | 1.000 | 1/1 |
| typo | 10 | 70 % | 90 % | 0.775 | – |
| phrase | 12 | 50 % | 75 % | 0.611 | – |
| all | 179 | 93 % | 98 % | 0.951 | 16/16 |

*Section* counts the questions with a heading whose hit opened at that heading.

#### OpenSearch 3.10.0 (2026-11-23)

| opensearch | questions | hit@1 | hit@5 | MRR | section |
|---|---|---|---|---|---|
| code | 65 | 100 % | 100 % | 1.000 | – |
| code-variant | 32 | 100 % | 100 % | 1.000 | – |
| code-absent | 7 | 100 % | 100 % | 1.000 | – |
| title | 18 | 100 % | 100 % | 1.000 | – |
| heading | 15 | 93 % | 100 % | 0.967 | 15/15 |
| wordform | 10 | 100 % | 100 % | 1.000 | – |
| compound | 10 | 80 % | 90 % | 0.867 | 1/1 |
| typo | 10 | 90 % | 100 % | 0.950 | – |
| phrase | 12 | 50 % | 83 % | 0.663 | – |
| all | 179 | 94 % | 98 % | 0.964 | 16/16 |

OpenSearch analyses German text with a light stemmer (`light_german`), the German normalisation and the same stop words.
Title, heading and text have a second field of word parts (n-grams of four and five letters) for compounds. It searches
every word as written, some words, words with one or two typing errors (`AUTO:5,9`, as Meilisearch) on a third field of
the words unstemmed, the last word as a prefix and the word parts, weighted title 8, heading 4, tags 3, text 1. Variants, MRR of all 179 questions:

| Variant | wordform | compound | typo | phrase | all |
|---|---|---|---|---|---|
| no word parts | 0.900 | 0.863 | 0.900 | 0.663 | 0.956 |
| word parts 4–5, 80 % of them must match | 1.000 | 0.864 | 0.950 | 0.694 | 0.966 |
| word parts 4–5, 60 %, typing errors on the stems | 1.000 | 0.864 | 0.950 | 0.747 | 0.970 |
| **the same, typing errors on the words unstemmed (chosen)** | 1.000 | 0.867 | 0.950 | 0.663 | **0.964** |
| the same, typing errors on both | 1.000 | 0.864 | 0.950 | 0.622 | 0.961 |
| word parts 4–5, 80 %, weight 1 | 1.000 | 0.870 | 0.950 | 0.694 | 0.961 |
| word parts 3–5, 80 % | 1.000 | 0.864 | 0.933 | 0.652 | 0.963 |

Typing errors on the stems scored best on the corpus, but missed a typing error in a short word: the stemmer cuts `Wagen`
to a stem too short for one, so `Wafen` found nothing (a test of the shared suite now). The chosen variant gives up 0.006
of MRR, all of it in the questions in many words, for that.

Not measured: the `hyphenation_decompounder` and `hunspell` need a German hyphenation grammar or dictionary installed in
the cluster, which the image does not bring.

#### The two compared

| | Meilisearch v1.56.2 | OpenSearch 3.10.0 |
|---|---|---|
| codes (104 questions, hard gate) | 100 % first | 100 % first |
| MRR of all 179 questions | 0.951 | 0.964 |
| word forms (`Wareneingänge buchen`) | 0.808 | 1.000 |
| compounds (`Warteschlange`) | 1.000 | 0.867 |
| typing errors | 0.775 | 0.950 |
| questions in words | 0.611 | 0.663 |
| memory in the tests | about 100 MB | 512 MB heap, a JVM |
| licence | MIT | Apache-2.0 |

Both are good enough for German: every code comes first, and nine of ten questions find their page first. OpenSearch is
better with word forms, typing errors and questions in many words, because it stems German; Meilisearch is better with
compounds, because it splits them, and needs a fraction of the memory and no tuning. Meilisearch stays the default
(ADR-24); a hoster who runs OpenSearch already, or whose users ask in sentences, chooses OpenSearch.

How the settings were chosen, MRR of all 179 questions (codes were 100 % in every variant, they do not depend on these
settings):

| Variant | wordform | compound | typo | phrase | all |
|---|---|---|---|---|---|
| no locale | 0.833 | 0.729 | 0.436 | 0.444 | 0.890 |
| German locale (compounds split) | 0.764 | 1.000 | 0.675 | 0.458 | 0.932 |
| German, title and heading also unsplit | 0.775 | 1.000 | 0.775 | 0.458 | 0.939 |
| German, stop words | 0.798 | 1.000 | 0.675 | 0.611 | 0.944 |
| **German, unsplit title and heading, stop words (chosen)** | 0.808 | 1.000 | 0.775 | 0.611 | **0.951** |
| the same, matching strategy `frequency` | 0.950 | – | 0.550 | 0.590 | 0.945 |
| the same, exactness before attribute | 0.808 | – | 0.686 | 0.611 | 0.939 |

The German locale splits compounds, but also words like `Inventur`, which then no longer match `Inventor` with one typing
error. Title and heading stand in the index a second time unsplit, which brings the typing errors back where they matter
most. What stays weak are questions in many words that share few with the page (`Kunde will Geld zurück` for *Retouren
annehmen*): the semantic search of M-13 is meant for them.
