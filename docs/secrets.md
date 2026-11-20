# Passwords in the text

A page can hold passwords, keys and other short secrets that the people who run something need: the admin password of a
switch, the key of a wireless network ([ADR-7](../backlog/ADR-7-passwords-outside-markdown.md)). The text of the page
holds only a reference, `::secret{id="7k2m9q4x1c"}`; the value lives sealed in the database and comes from the server on
a click only, and every time is logged.

> **This is operations documentation, not a vault.** Whoever runs the server and holds its key can read every value.
> There is no end-to-end encryption, no sharing outside the workspace, no password generator and no browser integration.
> For passwords that matter more than the documentation around them, use a password manager and write down where to
> find them.

## In the workspace

- **Writing:** "Password" in the slash menu or the bar of the editor asks for the value in a field that does not show it,
  makes the secret and writes its reference where the cursor is, also inside a sentence.
- **A password typed as text** (2026-11-20): a line such as `Router password: hunter2`, or a row of a table with such a
  label, holds the whole page back in the editor before anything is sent: the value is underlined, the status says "Not
  saved: is this a password?", and below the line **Keep it as a password** makes a secret from the value and puts its
  reference in its place, **It is not a password** saves the text as typed. Labels end in password, passwort or kennwort,
  or are passphrase, pw, pwd, pin, secret, geheimnis, token or api key; the value is one word of 4 to 200 characters,
  not a placeholder such as `***` or `<password>`. Code (fenced, indented, a label in inline code) and comments never
  ask, nor does a value the stored block already holds. While the cursor is on such a line with a value of 1 to 3
  characters, nothing is saved, so the first letters never reach a version. Closing the editor before answering leaves
  the value out (`…`) and saves the rest. Only the editor asks: an import, the API of blocks and the files of Files do not
  (ADR-7).
- **Reading and writing alike**, the reference shows as a mask, `••••••••`, with **Reveal**, which shows the value in
  place until the page is drawn again, and **Copy**, which puts it on the clipboard without showing it.
- **Hidden:** whoever reads the page but may not reveal this secret sees "Password hidden".
- **Not available here:** a reference outside the page of its secret, one whose secret was deleted, and one that names
  nothing look alike, so a copied or guessed ID tells nothing.
- **The secrets of a page** are listed in its settings, each with when and by whom it was last changed, whether the text
  still refers to it, its versions (to restore one), who may reveal it, and deletion. Deleting the reference from the text
  does not delete the secret, so undo and the versions of the block still find it.

## Who may do what

Rights come from the page the secret belongs to ([rights.md](rights.md)), never from where the reference is shown.

| Who | May |
|---|---|
| Whoever reads the page (read, comment, edit, manage, owner) | reveal and copy |
| Whoever edits the page and may reveal the secret | make secrets, change a value, see and restore versions, delete |
| Whoever manages the page | restrict a secret to persons and groups, or lift the restriction |

- **A restriction** narrows: only the persons and groups named, and of those only who also read the page, reveal it; the
  owner and the managers too only if they are named. Managers can name themselves, and the log keeps it. A group that is
  deleted leaves the restriction.
- **A role lent by a share link** to comment or edit counts for nothing here: secrets never reach a share.
- **Administrators** manage rights, not content: they reveal nothing where they hold no role.
- **In the trash** a secret reveals nothing; restored with its page it does again; a page purged and a tenant deleted take
  their secrets with them.

## Where a secret goes, and where not

- **Embedding** a block or page shows its secrets as on their own page; the reveal asks the page of the secret, so an
  embedding grants nothing.
- **Copies** never take values: a copied page or block, a template, a part kept as a copy for each use (ADR-22) and an
  import with IDs from elsewhere keep the references, which show "not available here" there.
- **Moving a block** to another page takes its secrets along with their versions and restriction, if whoever moves it may
  reveal each of them; otherwise the move is refused. Moving a page keeps its secrets, and who reveals them changes with
  the roles of the page.
- **Never:** the search (the reference is stripped, the value is never in the text), exports of the web and of the command
  line (they keep the references), the list of problems, the versions of the page, frozen pages of share links ("Password
  not shared", without the ID), the working copy of Samba (pages are not on the share), and the logs of the process.

## Revealing

```text
GET    /api/pages/<id>/secrets                  the secrets of a page, with what the viewer may do; "vault": false without a key
POST   /api/pages/<id>/secrets                  {"value"}   → {"id", "ref"}; edit on the page
GET    /api/secrets/<id>                        one secret, with readers and versions as far as the viewer may know them
POST   /api/secrets/<id>/reveal                 {"purpose": "show|copy", "version": n}   → {"value"}
PUT    /api/secrets/<id>                        {"value"}   a new version
POST   /api/secrets/<id>/versions/<n>/restore   the value of version n as a new version
PUT    /api/secrets/<id>/readers                {"restricted": true, "readers": [{"kind": "person|group", "who": "<id>"}]}
DELETE /api/secrets/<id>                        the secret and every version
```

The reveal is the only answer that carries a value: a `POST` from a page of this application, for a signed-in account,
answered with `Cache-Control: no-store`, at most 20 times a minute per account (then `429` with `Retry-After`), and only
after the log took it down in the same transaction. A secret one may not reveal answers `404`, the same as one that does
not exist. A value has 1 to 16384 characters. The limit per minute is counted in the log, in the transaction that
logs the reveal, under a lock per account (2026-11-20), so several instances behind one address share it and a restart
does not reset it; the memory of each instance turns a burst away before the database and alone limits what lies in
something private, which is not logged.

## The log

Making, revealing, copying, changing, restoring, restricting, moving and deleting a secret are each logged on its page
(`secret made`, `secret revealed`, `secret copied`, …), with the ID of the secret and the version where one was named,
never a value; the tab Rights shows them like every entry ([rights.md](rights.md#the-log)), kept 365 days unless the
administrators set 30 to 3650. Nothing of a private page is logged, as for everything private. A move is logged on both
pages, so the page a secret leaves keeps the trace: it names the page it went to, or says only "into something private".

## The key

Values are sealed with AES-256-GCM of the Go standard library under `UNDERDOC_SECRET_KEY`, the same vault that seals TOTP,
the client secret of OIDC and the NT hashes of Samba ([sign-in.md](sign-in.md#secrets-of-other-systems)). Each version is
bound to its tenant, its secret and its number, so a sealed value copied to another row does not open.

- **Without a key** secrets are neither made nor revealed; the editor and the API say so, and nothing falls back to plain
  text. **A wrong key** reveals nothing.
- **A backup of the database** holds the values sealed only. Keep the key apart from the backups: either alone is
  useless, both together are everything.
- **Rotating the key:**

  ```bash
  openssl rand -base64 32                           # the new key
  # restart UnderDoc with UNDERDOC_SECRET_KEY=<new> and UNDERDOC_SECRET_KEY_OLD=<old>: everything still opens
  underdoc rekey                                    # every sealed value of every tenant sealed again under the new key
  # restart without UNDERDOC_SECRET_KEY_OLD
  ```

  `underdoc rekey` takes the passwords of pages with every version, TOTP, the secrets of other systems and the Samba
  hashes, one transaction per tenant. A value that opens with neither key stops it, and that tenant keeps everything as it
  was. It can run again; what the new key already opens is left alone.

The tables are `page_secrets`, `page_secret_versions` and `page_secret_readers` (migration 028), under row-level security
like all others; the database function `secret_may` decides who reveals.
