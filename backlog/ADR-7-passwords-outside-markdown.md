---
id: ADR-7
type: Decision
title: Passwords never enter Markdown
status: erledigt
tags: [security]
created: 2026-11-06
---

## Context

Passwords should be writable in a page, hidden and revealed on click. Markdown sits on the share, in search, in versions, exports and backups.

## Options

- **Masked text in Markdown:** simple, but clear text everywhere.
- **Server-side encrypted store with a reference in the text:** convenient, but whoever holds server and key can read it.
- **End-to-end encryption with groups:** strongest, but a cryptography project of its own.

## Decision

The text holds only `::secret{id="..."}`. Values are encrypted in the database with a maintained library and a key from the environment. Each password has its own permission, values arrive from the server on click only, and every reveal is logged.

## Consequences

- Never included in search, exports, reports, public shares or the API.
- This is operations documentation, not a vault, and the documentation has to say so.

## Settled on 2026-11-20: how it is built

Decided before the code, in the sprint of that day; the maintainer asked for ADR-7 to come with the Samba part.

**Where values live.** A password of the text is a **secret of a page**: its own row with a random ID of the form of ADR-18
in a table of its own (`page_secrets`), bound to exactly one page, with every value it ever had as a version (who, when),
each sealed on its own. The vault of ADR-23 seals them (AES-256-GCM of the Go standard library, `UNDERDOC_SECRET_KEY`),
bound to the tenant, the secret and the number of the version, so a sealed value copied to another row, version or tenant
does not open. There is no second key: one key, one vault, one way to rotate it. The text holds `::secret{id="…"}` and
nothing else, anywhere in a block, also inside a paragraph.

**Who reveals.** Rights come from the page the secret belongs to, never from where its reference is shown:

- By default, **whoever reads the page** (read and above) may reveal and copy its secrets. A role lent by a share link to
  comment or edit does not count: secrets never reach a share.
- **Narrower on request:** whoever manages the page may restrict a secret to persons and groups. Then only those of them
  who also read the page reveal it; everyone else sees "hidden". There is only narrowing, as with roles; managers can
  add themselves, and the log keeps it.
- **Changing, deleting and the versions** of a secret need edit on the page and the right to reveal it. Making a secret
  needs edit on the page; it starts unrestricted.
- Administrators manage rights, not content (ADR-29): they reveal nothing where they hold no role.
- What lies in the trash reveals nothing; a page purged, and a tenant deleted, take their secrets with them.

**Shown where.** A reference shows the secret only in a block of its own page, directly or embedded elsewhere; an
embedding grants nothing, since the reveal asks the page of the secret. Everywhere else, a reference shows "not available
here", the same for an ID that does not exist, belongs to another page or another tenant, so a guessed or copied ID
learns nothing. This makes copies, templates, imports with foreign IDs and the copy of a part kept for each use (ADR-22)
safe without rules of their own: **values never travel with a copy.** The references stay in the copy and show "not
available here" until someone writes new secrets there.

**Moving a block to another page** takes its secrets along, with their versions and restriction, if whoever moves it may
reveal each of them; otherwise the move is refused. Moving a page keeps its secrets; their readers change with the page,
as its roles do, which needs manage (ADR-29).

**On click only.** Pages, embeddings, the API of documents, exports and frozen links carry the reference, never a value.
The value comes only from `POST /api/secrets/<id>/reveal`, for a signed-in account, from the same origin, answered with
`Cache-Control: no-store`, at most a few each minute per account, and written to the log before it is answered: if the
log cannot be written, nothing is revealed. Copying without showing uses the same request with the purpose "copy" and
puts the value on the clipboard without drawing it.

**The log** (ADR-29) names making, revealing, copying, changing, restoring, restricting, moving and deleting a secret,
each time, on its page, with the ID of the secret and never a value, and keeps the entries as long as every other entry
(365 days unless the administrators set 30 to 3650). Nothing of a private page is logged, as for everything private.

**In the editor** (ADR-21): "Password" in the slash menu and the bar asks for the value in a field that does not show
it, makes the secret and writes the reference at the cursor. The live preview and the reading view show a mask with
"Reveal" and "Copy"; whoever may change it also gets "Change". Deleting the reference from the text does not delete the
secret, so undo and the versions of the block still find it; the secrets of a page are listed in its settings, with
their versions, restriction and deletion.

**Never included:** the search index (the directive is stripped, the value is never in the text), exports of the web and
the command line, the list of problems and other reports, frozen pages of share links (a mask that says "not shared",
without the ID), the versions of the page, the working copy of Samba (pages are not on the share; a Markdown file of Files
is a file and holds only what was typed into it), and the logs of the process: a value is a `vault.Secret` that prints as
a mark.

**Key.** Without `UNDERDOC_SECRET_KEY`, secrets can be neither made nor revealed; the page says so, and there is no plain
fallback. A wrong key reveals nothing. **Rotation:** start with the new key in `UNDERDOC_SECRET_KEY` and the old one in
`UNDERDOC_SECRET_KEY_OLD`, so everything still opens, then `underdoc rekey` seals everything of every tenant again under the
new key (the secrets of pages and their versions, TOTP, the client secret of OIDC, the NT hashes of Samba); it can be run
again, and then the old key goes. A backup of the database holds the values sealed only, and is useless without the key;
keep the key apart from the backups.

**Not a vault.** The documentation says plainly: this is operations documentation. Whoever runs the server and holds the
key can read every value; there is no end-to-end encryption, no sharing outside the tenant, no password generator and no
browser integration.

**Built on 2026-11-20** ([docs/secrets.md](../docs/secrets.md), migration 028): as settled above. The mask, Reveal and
Copy while reading come from the server and `secrets.js`; the editor draws its own masks with Change, and "Password" in
the slash menu and the lock in the bar make a secret. The settings of a page list its secrets with versions, readers and
deletion. Reveals are limited to 20 a minute per account. `underdoc rekey` and `UNDERDOC_SECRET_KEY_OLD` rotate the key
of the whole vault. Open: ~~a warning when Markdown looks like it holds a password (concept)~~ (built 2026-11-20, below), and the attack
round of M-5.

**After the attack round (2026-11-20):** moving a block whose secret goes along is logged on both pages, so the page a
secret leaves keeps the trace even when it goes into something private, which logs nothing; that entry then says "into
something private" and names no private page. ~~A known limit, not changed: the limit of reveals per minute is kept in the
memory of each instance, so several instances behind one address multiply it, and a restart resets it; the log still
takes down every reveal.~~ (until 2026-11-20, below: the log counts them.)

## Settled on 2026-11-20: a password typed as text

Decided before the code, in the sprint of that day. Someone types `Router password: hunter2` into a page instead of
using "Password". Once saved, the value is in the block, its versions, the search index and the log of what changed,
and no later step takes it out of all of them. So the warning has to come **before** the save, in the browser.

- **Where:** the editor of a page (ADR-21), at every save. Only what is new counts: a value the stored block already
  holds, from before this warning or because its author said so, does not ask again.
- **What looks like a password:** a line whose label ends in password, passwort or kennwort (also inside a word, as in
  adminpasswort) or in the word passphrase, pw, pwd, pin, secret, geheimnis, token, api key or api-key (any case, with
  or without bold, italics, a list mark or a quote; at most four words), then `:` or `=`, or a row of a table with such
  a label in one cell and the value in the next, and then **one word** of 4 to 200 characters. Not a value: a word with a space in it ("ask Bob"), a reference `::secret{…}`, a link, and
  placeholders such as `***`, `…`, `xxx`, `<password>`, `{{password}}`, `-`, `n/a`, `none`, `see below`.
- **No false alarms in code:** fenced code, indented code and HTML comments (the markers of blocks and parts) are never
  looked at, nor a label in inline code, so a configuration file shown in a page, or a command line with
  `--password=…`, does not ask. A value in inline code after a label outside it (``Password: `hunter2` ``) does.
- **Held, not sent:** while a value waits for an answer, the editor saves nothing of the page: no block, no version,
  nothing for the search or the log leaves the browser. The status says "Not saved: is this a password?", and below the
  line the editor asks: **Keep it as a password** makes a secret of the page from that value (`POST
  /api/pages/<id>/secrets`, the same as "Password") and puts its reference in place of the value, then saves; **It is
  not a password** saves the text as typed. Changing the text again asks again for what is then new. The value stays in
  the memory of the browser alone; nothing writes it to the storage of the browser.
- **While typing:** autosave runs a second after the last key. A value of one to three characters after a label is not
  asked about, but while the cursor is on its line nothing is saved either, so the first letters of a password never
  reach a version. A short value the cursor left is saved.
- **Closing the editor** while a value waits for an answer leaves the value out (`…` in its place), saves everything
  else, and says so; the value never leaves the browser.
- **Not covered:** an import, the API of blocks used by something else than the editor, and a Markdown file in Files or
  on the share, which is a file and holds what was typed into it (above). The warning helps people who type; it is not
  a filter the server enforces, since the server would only see the value once it is too late.

**Built on 2026-11-20** ([docs/secrets.md](../docs/secrets.md), `web/editor/src/plain.js`): as settled above.

**The limit of reveals across instances (2026-11-20):** the known limit above is lifted where it was cheap. A reveal
counts the reveals and copies of the account in the log of the last minute, in the same transaction that logs it, under
a lock per account, so instances behind one address share the limit and a start does not empty it. The memory of each
instance still turns a burst away before the database, and it alone limits what lies in something private, which is
not logged.
