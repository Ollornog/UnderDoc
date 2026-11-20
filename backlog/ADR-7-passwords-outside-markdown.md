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
