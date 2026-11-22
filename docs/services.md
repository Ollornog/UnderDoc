# Services next to UnderDoc

UnderDoc is one binary with PostgreSQL. A few things it does better with a service next to it, each optional:
[`deploy/compose.yml`](../deploy/compose.yml) starts them with Docker Compose, pinned by tag and digest. UnderDoc itself
and PostgreSQL are not in that file: they run however an installation runs them (see the README).

| Service | For | Profile | Port (loopback only) | UnderDoc settings |
|---|---|---|---|---|
| Meilisearch | The search (ADR-24, [search.md](search.md)), the default engine | always | 7700 | `UNDERDOC_SEARCH_URL=http://127.0.0.1:7700`, the key in the file named by `UNDERDOC_SEARCH_KEY_FILE` or in `UNDERDOC_SEARCH_KEY` |
| Mailpit | Catching every mail in development and tests (M-7, M-22); it delivers nothing | always | 1025 (SMTP), 8025 (web) | none yet: UnderDoc sends no mail so far |
| OpenSearch | The search, as the choice of ADR-24 | `opensearch` | 9200 | `UNDERDOC_SEARCH_ENGINE=opensearch`, `UNDERDOC_SEARCH_URL=http://127.0.0.1:9200` |
| Gotenberg | Previews of Office files ([files.md](files.md)) | `office` | 3000 | `UNDERDOC_OFFICE_URL=http://127.0.0.1:3000` |

```bash
cd deploy
printf 'MEILI_MASTER_KEY=%s\n' "$(openssl rand -hex 24)" > .env && chmod 600 .env
docker compose up -d                                  # Meilisearch and Mailpit
docker compose --profile opensearch --profile office up -d   # and the optional ones
```

UnderDoc reads the key of Meilisearch from a file only UnderDoc can read (mode 0400), named in `UNDERDOC_SEARCH_KEY_FILE`,
or, where it runs in a container that gets its settings from a `.env` file, from `UNDERDOC_SEARCH_KEY` (a read-only
container cannot take a Compose secret from the environment). After the search service is set up, or after it
lost its data, `underdoc reindex` fills it again; a new index is filled by itself when `serve` starts.

**Rules the file keeps:**

- Every port listens on loopback only. Nobody but UnderDoc talks to these services; the browser asks UnderDoc.
- Secrets come from `.env` next to the file, never from the file itself. Compose refuses to start Meilisearch without
  `MEILI_MASTER_KEY`.
- OpenSearch runs without its security plugin, as in the tests: no user, no TLS, reachable only on loopback and in the
  network of these services. For anything beyond a test, switch the plugin on with a certificate the system trusts and a
  user with the rights on the indexes `<prefix>-*` ([search.md](search.md)), and set `UNDERDOC_SEARCH_USER`.
- The network has a fixed name, `underdoc-services`. UnderDoc running in a container of another Compose project joins it
  (`networks: {underdoc-services: {external: true}}`) and reaches the services by their names, for example
  `http://meilisearch:7700`; `127.0.0.1` inside a container is the container itself.
- The search engines run the versions the tests use (`scripts/search/Dockerfile.*`); `tests/test_repo.py` holds the
  file to them, and Dependabot moves them there. `scripts/check-services.sh` (in the CI) starts the services from this
  file: Meilisearch has to take over a database of the version before, Gotenberg has to turn a spreadsheet into a PDF.
  Mailpit has no test, so updates of this file are still looked at by hand.

## Updating Meilisearch

A Meilisearch database belongs to the version that wrote it: a newer image refuses to start on it ("Your database
version … is incompatible"), and the search falls back to titles. The file sets `MEILI_UPGRADE_DB: "true"`, so a new
image upgrades the database in place when it starts (`--upgrade-db` since v1.51, the page "Updating" of the
Meilisearch documentation). On a database that is already current, or a new one, it does nothing, so it stays on.
Databases older than v1.12 cannot be upgraded this way.

The upgrade is **not atomic**: if it fails halfway, the database can be broken. Take a snapshot first:

```bash
cd deploy
docker compose stop meilisearch
docker run --rm -v underdoc-services_meilisearch:/meili_data:ro -v "$PWD":/backup debian:trixie-slim \
    tar -C /meili_data -czf /backup/meilisearch-before-update.tar.gz .
docker compose pull meilisearch && docker compose up -d --wait meilisearch
```

If the new version does not get healthy, stop it, restore the archive into the volume and go back to the image before.
The index holds nothing that is not in UnderDoc: an empty volume works too. When `serve` starts, it finds the index
missing and fills it again (`underdoc reindex` does the same by hand), so restart UnderDoc after the search service
lost its data.
