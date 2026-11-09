# Export and import

Pages live in the database as blocks (ADR-19), and Markdown stays the way in and out. `underdoc export` writes every
collection as normal files at any time (ADR-13, the way out), and `underdoc import` reads such files, or any folder of
Markdown files like the folder source, into a new collection.

```bash
export UNDERDOC_DATABASE_URL=postgres://underdoc@db/underdoc

underdoc export ./backup                               # a new or empty directory, nothing is overwritten
underdoc import --owner anna ./backup/Team "Team"      # a new collection owned by the account with the login anna
```

The web offers the same files for one collection, folder or page as a zip: "Export as files" in the right click menu of a
collection, or `GET /api/objects/{id}/export`. It holds only what the viewer sees, and at most 5000 objects.

## Layout

```text
backup/
  .underdoc.yaml                format, and the order of the collections at the top
  Team/                         a collection
    .underdoc.yaml              id, kind, title, description, access, and the order of what lies in it
    Network plan.md             a page
    Network plan/               the children of that page, beside it under the same name
      .underdoc.yaml            only the order
      Cables.md
    Archive/                    a folder
      .underdoc.yaml
  private/                      private notes, only in the export of the command line
    anna/                       one directory per login
      My notes/
        .underdoc.yaml          says private: true
```

- **Collections and folders** are directories. Their `.underdoc.yaml` holds what a directory cannot: the ID, the kind
  (`collection` or `folder`), the exact title, the description and access level where set, and the order of the entries,
  lines and spaces included, each with its ID:

  ```yaml
  id: 7k2m9q4x1c
  kind: collection
  title: 'Team: operations'
  access: read
  order:
      - name: Network plan.md
      - line: 3h7k9m2x4c
      - name: Archive
      - space: 9x4c1m2k7q
  ```

- **A page** is a Markdown file with front matter: `id`, `title`, and `description`, `access` and `tags` where set. Its blocks
  follow in order, each after a marker that keeps its ID:

  ```markdown
  ---
  id: 8p3n5v2w6h
  title: Network plan
  next_block: 6
  ---

  <!-- block b1 -->
  ## Switches

  Two of them, in the tower.

  <!-- block b3 -->
  ::embed{ref="8p3n5v2w6h#b1"}
  ```

  The marker is an HTML comment on a line of its own, so every Markdown viewer, and UnderDoc itself, leaves it out. A block
  is written as it is, directives such as `::embed{…}` included; only blank lines at its start and end are dropped, as
  they mean nothing in Markdown and the file needs one between blocks. `next_block` is written only when a block at the end
  was deleted, so that its ID is not given out again after an import.

- **Names** of files and directories are only labels; the `id` in the file is the identity. A name is the title with the
  characters that Linux or Windows refuse replaced by `_`, without a leading dot or a trailing dot or space, never a device
  name of Windows such as `CON`, at most 80 characters, and unique in its directory without regard to case: a second
  "Plan" becomes "Plan (2)". A page and its children's directory share one name.
- **Private notes** are seen in the web only by their owner (ADR-20), also not by administrators. The command line runs on
  the server, where whoever runs it can read the database anyway, so a full export takes the private notes of every
  account into `private/<login>/`, to keep a backup whole. Guard that directory like the database. The zip of the web
  holds only what the viewer sees, which never includes someone else's private notes.
- What lies in the trash is not exported. Versions of blocks are not exported either: an export is the current state.

## Import

- The directory becomes a new collection at the top, with the title given on the command line, owned by the account given
  with `--owner` (it must have signed in once). Its `.underdoc.yaml`, if any, gives the description, access level, order
  and, where it says so, privacy: imported private notes stay private to their new owner.
- Directories become folders, or collections if their `.underdoc.yaml` says so. A directory beside a page with the same
  name, without a kind of its own, holds the children of that page. Entries starting with a dot, symbolic links and
  anything that is not a regular file are left out. Files that are not Markdown are named in the warnings and left out
  for now: files that are files come with the content store.
- **IDs:** an object keeps its ID where it has the form of ADR-18 and nothing in the database has it yet, the trash
  included; otherwise it gets a new one, and the import names each. Nothing that exists is ever changed. So an export
  imported into an empty database comes back with every ID, and imported into the same database again it becomes a copy.
  IDs of the folder source such as `network-plan` do not have that form and get new ones.
- **Links follow the pages** (2026-11-09): a relative link to a Markdown file of the tree and a link `[[page]]` by ID,
  path, file name or title become a link `/o/<id>` to the page it became, with its anchor; `/o/<id>` links and
  embeddings of objects of the tree that got a new ID follow it too. A link that finds nothing, and anything in fenced
  code, stays as written, and the list of problems names it.
- **Blocks:** the markers give the blocks and their IDs. An ID is kept where it is valid and not used before in the same
  page; every other block gets the next number after the highest, and the next block of the page after that comes later
  still, never before `next_block`. A marker inside fenced code is text. A page without markers is split before each
  heading of level 1 to 3 outside fenced code, and a first heading of level 1 that only repeats the title is left out,
  since the page shows its title. A block longer than 100000 characters is cut, at blank lines where it can.
- The title of a page is its `title`, else its first heading of level 1, else its file name. Tags come from `tags` in the
  front matter or the `.underdoc.yaml`, as a list or a line with commas, and are kept as the database keeps them; tags it
  cannot keep are named in the warnings and left out. The export writes them the same way (2026-11-09).
- The whole import is one transaction: it comes in whole or not at all.
- The export warns about a block that would not come back exactly, for example one that holds a line like a marker or a
  code fence it never closes.
