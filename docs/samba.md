# Samba

Files of the view **Files** as a network drive on Windows, macOS and Linux, over SMB ([ADR-30](../backlog/ADR-30-samba-share.md),
ADR-28, ADR-10). What is saved there becomes a version in UnderDoc, and what changes in the web appears on the share.
UnderDoc never runs as root: a **companion** container runs smbd, and UnderDoc steers it over a Unix socket.

## What is shared

- **Company** (`\\<server>\Company`): the folder of the tenant, once the tenant has a companion.
- **My files** (`\\<server>\<user name>`): the folder of each account that has an app password (with a domain: that turned
  SMB on), through `[homes]`, for that account alone.
- **Collections** whose managers switched **Share over Samba** on in their rights dialog, each a share named after it.
- **A folder in which someone holds a role without reading the folder above** is a share of its own for them, named after
  the folder, so they reach it without seeing what lies above (ADR-29).

Only shared libraries are on disk; the content store keeps every version either way. A library that stops being shared
leaves the disk after a last pass has taken what changed there into the database; if something there is not taken (its
writer unknown, or refused), the directory stays on disk, the log says what, and every pass looks again.

## Rights on the share

The actions of each account (ADR-36) become POSIX ACLs per folder: read where it may read, write where it may read,
create, write and delete, nothing where it may not read. Each account is decided on its own, so a deny on a folder holds
on the share as in the web. `hide unreadable`
hides what a user may not read, and `access based share enum` lists only the shares a user reaches. **Samba knows rights
per folder only** ([T-1](../backlog/T-1-measure-office-saving.md)): a save through a temporary file and a rename makes a
new file that takes the rights of its folder, so a right on a single file would not survive it. In a shared library a
allow on a single file holds in the web only, and the rights dialog says so; a deny on a single file and breaking its
inheritance are refused there (the share would show what the web hides), a folder can do both. For the same reason a
collection holding a file that breaks the inheritance or denies someone is not shared, and nothing holding one moves
into a shared library: both answer `409` naming the files, which must inherit again and lose their deny first. A file that is in a shared library anyway (it broke the inheritance before the tenant had a
companion, or came back from the trash) is kept off the disk, and the log says so.

An **archived** library ([rights.md](rights.md#archived-spaces), T-34) is read only on the share for everyone until it
is restored: the one check refuses create, write and delete there, so the ACLs give read only, and a new file or version
over SMB is refused like one without rights.

A file moved into another folder, in the web or over SMB, and a file saved through a rename get the ACL of their folder
at the next pass, and belong to UnderDoc's UID from then on.

## The app password

In the profile, **SMB share** makes an app password: the user name is the login when it fits (lower case letters,
digits, `.`, `_`, `-`, up to 32), otherwise one chosen there, and it stays. The password (20 characters in groups of
five) is shown once. A name that is the name of a share (`company` next to `\\files\Company`, in any case) is refused:
its `[homes]` would push the share aside. A share keeps its name anyway, should such a user exist. UnderDoc keeps only its NT hash, sealed by `UNDERDOC_SECRET_KEY`, so Samba needs a vault key. A new
app password replaces the old one and ends its sessions; removing it ends the Samba user. Administrators list the Samba
users and remove any.

```text
GET    /api/samba                       on, server, the viewer's Samba user and the shares it reaches
POST   /api/samba/password              {"name": "anna"}  → {"name", "password", "server"}, shown once
POST   /api/samba/domain                with a domain: the Samba user of one's domain account → {"name", "server"}
DELETE /api/samba/password              remove one's own (with a domain too)
GET    /api/samba/accounts              administrators: every Samba user
DELETE /api/samba/accounts/<account>    administrators
PUT    /api/files/roots/<id>/samba      {"on": true}   managers of a collection of the files
```

The facts panel of a file shows its path on the share (`\\files\Company\Contracts\plan.xlsx`) and who has it open there;
`GET /api/files/<id>` answers it as `share`.

## A domain (optional)

Instead of app passwords, the companion of a tenant can be a member of an Active Directory domain (T-42, ADR-30): people
sign in to the share with their domain account, by Kerberos from a computer of the domain or by NTLMv2 with its
password. Without the settings below everything stays as above.

- **One mode per tenant, chosen at setup, never mixed** (decided by the maintainer on 2026-11-24). With a domain only
  domain accounts reach the share: there are no app passwords, the companion keeps no local password at all, and a
  sign-in as a local user of the companion (`FILES\anna`) fails. The companion maps a domain account to a UID by name,
  so a local user of the same name would be the same person to the kernel. UnderDoc refuses to start while a tenant
  holds Samba users of the other mode, and the companion refuses a state of the other mode; changing the mode is a
  migration (below).
- **Who reaches the share.** In the profile, **SMB share** offers **Use my domain account**: the account gets a Samba user
  named like its domain account and a UID of the range, without a password. The name comes from the setup, never from
  the account: its login in lower case, or the lasting ID the gate gives it (`UNDERDOC_SAMBA_DOMAIN_NAMES`). Choose the
  way that matches the names of the domain; the login fits where the gate takes its accounts from the same directory.
  Stopping SMB removes the Samba user, as removing an app password does.
- **What the companion lets in.** winbindd maps a domain account to the UID of the Samba user of the same name
  (`idmap_nss` for the domain, within `UNDERDOC_SAMBA_UIDS`); the image has no `libnss-winbind`, so the users of the
  system are only those UnderDoc applied. A domain account without a Samba user (one never turned on, or removed), a
  user of the system (`root`, `daemon`, …, whose UIDs lie outside the range) and the administrator of the domain get no
  UID and are turned away with the right password. Upper case (`EXAMPLE\DORA`) is the same account. Whenever the users
  change, the companion empties winbindd's caches, so a UID that passes to another account is never reached by the
  domain account that had it before, and an account turned on just now is let in at once.
- **Rights, audit and everything else** are as with app passwords: the Samba user is the account, and its name is what
  smbd's audit and the open files name.

| Setting of UnderDoc | Meaning | Default |
|---|---|---|
| `UNDERDOC_SAMBA_DOMAIN` | Tenants of `UNDERDOC_SAMBA` whose companion is a member of a domain, comma separated | none: app passwords |
| `UNDERDOC_SAMBA_DOMAIN_NAMES` | What names the domain account of an account: `login` or `remote-id` | `login` |

| Setting of the companion | Meaning | Default |
|---|---|---|
| `UNDERDOC_SAMBA_DOMAIN_REALM` | The realm of the domain, such as `EXAMPLE.TEST`; switches the companion to a member | none: app passwords |
| `UNDERDOC_SAMBA_DOMAIN_WORKGROUP` | The short name of the domain, such as `EXAMPLE` | required with a realm |
| `UNDERDOC_SAMBA_DOMAIN_JOIN_USER` | An account that may join computers to the domain; only needed while not joined | none |
| `UNDERDOC_SAMBA_DOMAIN_JOIN_PASSWORD_FILE` | A file holding its password (a secret of the container, mounted read only) | none |
| `UNDERDOC_SAMBA_DOMAIN_STATE` | A volume for the secret of the machine account (`secrets.tdb`) | `/var/lib/underdoc-samba-domain` |

- **The join.** At start the companion checks its membership (`net ads testjoin`). Not a member, it joins with the account
  and the password file (`net ads join`, the password read by Samba from the file, never on a command line or in the
  environment; a password in `UNDERDOC_SAMBA_DOMAIN_JOIN_PASSWORD` is refused), checks again, and only then starts
  winbindd and smbd. Not a member and no password file: it ends and says why. The secret of the machine account stays on
  the volume, so a new container is still a member and needs the password file no longer; remove it after the join.
  The join leaves the DNS of the domain alone (`--no-dns-updates`): give the name people type (`UNDERDOC_SAMBA_NAME`, in
  the domain's zone) an address record for the address that publishes port 445, so Kerberos finds it.
- **The container** needs the domain controllers as its DNS servers (`dns:` in compose) and reaches them on the ports of
  Kerberos, LDAP and SMB. Its other rights stay as above; it needs no further capability.
- **Changing the mode** is a migration by the operator: stop UnderDoc, remove every Samba user of the tenant
  (`underdoc samba-users --tenant <id> remove`, which the log keeps), set `UNDERDOC_SAMBA_DOMAIN` and the companion the
  other way, start both. People then turn SMB on again in their profiles.

## The working copy

Each library lies at `<UNDERDOC_SHARE_DIR>/<tenant>/<ID of its root>/…` with the real names of folders and files, and
each file and folder carries its ID in the extended attribute `user.underdoc.id`. UnderDoc watches the tree with inotify
and takes a change two seconds after the last event, once the file is no longer open for writing:

- a changed file becomes a version by whoever wrote it (from smbd's audit, else the Samba user that owns the new file on
  disk);
- a new file or folder becomes an object of whoever made it;
- a rename or a move renames or moves the object;
- a file without an ID where a file was is a new version of it, which is how Office and LibreOffice save (T-1);
- a deleted file or folder goes to the trash;
- temporary files (`~$*`, `*.tmp`, `.~lock.*#`, `Thumbs.db`, `desktop.ini`, `.DS_Store`, `._*`) never become objects.
  These patterns hold for files only: a folder named `Backup.tmp` is an ordinary folder. A file made in the web with such
  a name stays off the disk;
- a hard link (SMB2 makes one) ends at the next pass: the name UnderDoc knows keeps the object and its ID, every other name
  becomes a copy of its own, a new file of whoever made the link. Samba has no setting against hard links.

**A writer nobody names is nobody:** when neither smbd's audit nor the owner of the file on disk names who changed
something, the change is never taken as the owner of the object or of the library. A move, a rename, a deletion and a
save are put back from the database; something new stays on disk without becoming an object; the log says each (action
`samba`). The audit keeps only operations that change something (a listing or a read does not push them out), numbers
them, and a pass that finds events missing since the one before says so in the log.

Every change is made as the user who made it, through the same checks as the web. What the database refuses is put back
on disk (a rename, a move, a deletion) or stays there without a version (new content, for example over the quota). A
change in the web writes to disk through a temporary file and a rename. **While a file is open over SMB the web changes
nothing of it:** a new version, a restore, a rename, a move or a deletion answers `409` with who has it open. A full pass
runs at start, after every change the database announces, and once an hour; the table `share_files` keeps what disk and
database last agreed on, so a restart knows which side changed.

**The quota reaches the client** (2026-11-20, ADR-30). After each pass UnderDoc gives the companion the room of each
shared library that a limit binds: the quota of the library or of a user's own folder, the limit of the tenant
(`UNDERDOC_FILES_QUOTA`) or the storage the hoster allows, whichever leaves the least, versions counted. smbd asks for it
through `dfree command` (a link `dfree` to the helper in its directory), so Windows shows that room on the drive and
Explorer refuses a copy that does not fit before it starts; a library without a limit shows the disk, and nothing shows
more than the disk has. smbd does not refuse a write itself: a save that does not ask for room first still lands on
disk, and stays there without a version, as does a file larger than `UNDERDOC_FILES_MAX_SIZE`; nothing writes the
database's content over it. **Whoever saved it is told:** once in the log as theirs ("… over Samba was not taken: the
quota is full"), when the workspace opens (a notice), and in the profile under SMB share, until there is room and the
file is saved again, or it is gone.

## Setup

UnderDoc and the companion share two directories at the same paths: the share directory, on a file system with extended
attributes and POSIX ACLs (ext4, XFS, btrfs; not NFS), and a directory for the socket. With many tenants each tenant that
wants SMB gets a companion of its own on its own directory and address; WebDAV is the door for the rest (M-7).

| Setting of UnderDoc | Meaning | Default |
|---|---|---|
| `UNDERDOC_SAMBA` | Tenants with a companion and their sockets: `main=/run/underdoc-samba/helper.sock`, comma separated | none: no Samba |
| `UNDERDOC_SHARE_DIR` | The share directory, the same path in every companion | none |
| `UNDERDOC_SAMBA_SERVER` | The name of the server as people type it, for share paths | none |
| `UNDERDOC_SAMBA_UIDS` | The UIDs of Samba users, within an unprivileged container | `20000-29999` |

Samba needs the database, the content store (`UNDERDOC_STORE_DIR`) and the vault (`UNDERDOC_SECRET_KEY`). Pages of the documents are not on the share, so neither are
their passwords ([secrets.md](secrets.md)).

### The companion

The image is built from [`deploy/samba/Dockerfile`](../deploy/samba/Dockerfile): smbd of Samba 4.24 from Debian 13
backports, the newest series a stable distribution carries, winbindd for a domain, and the helper `underdoc-samba`. An example is
[`deploy/samba/compose.yml`](../deploy/samba/compose.yml).

| Setting of the companion | Meaning | Default |
|---|---|---|
| `UNDERDOC_UID`, `UNDERDOC_GID` | The UID and GID UnderDoc runs as: the only peer of the socket, and the owner of every path | `65532` |
| `UNDERDOC_SAMBA_ROOT` | The directory of its tenant, `<UNDERDOC_SHARE_DIR>/<tenant>` | required |
| `UNDERDOC_SAMBA_SOCKET` | The socket | `/run/underdoc-samba/helper.sock` |
| `UNDERDOC_SAMBA_NAME` | NetBIOS name and server string | `UNDERDOC` |
| `UNDERDOC_SAMBA_UIDS` | The range of UIDs, the same as UnderDoc's | `20000-29999` |
| `UNDERDOC_SAMBA_AUDIT` | `all` logs every operation of smbd, for recordings like T-1 | the changing ones |

- **Volumes:** only `<UNDERDOC_SHARE_DIR>/<tenant>` at the same path, and the directory of the socket. Its own state (users,
  passwords, shares, smbd's databases) lives in `/var/lib/underdoc-samba`, a `tmpfs`: a new companion gets everything
  from UnderDoc at the next pass.
- **Rights:** it starts as root in its container, since smbd switches to each user's UID, with only `CHOWN`,
  `DAC_OVERRIDE`, `FOWNER`, `SETUID`, `SETGID` and `KILL`, `no-new-privileges` and a read-only root file system.
- **Port 445** only on an address of the LAN or the mesh, never on the internet. SMB 2.02 and later, NTLMv2 only, no guest,
  no SMB1, no POSIX extensions, no extended attributes from clients, signing where the client offers it.
- **Healthcheck:** `underdoc-samba probe` asks the helper, which answers only while smbd runs.

### The channel

The helper makes the socket with mode 0600 for `UNDERDOC_UID` and checks the peer of every connection (`SO_PEERCRED`);
anyone else is turned away. A connection carries one JSON request from a closed list, checked before anything happens:

| Request | Does |
|---|---|
| `ping` | answers while smbd (and with a domain winbindd) runs, with the versions, its mode (`local` or `domain`) and a name of this start: a companion that started again gets its state at the next pass, one of the other mode gets nothing |
| `apply` | the whole state: its mode, users (name, UID, NT hash, home; with a domain no hash) and shares (name, path, users); writes them, reloads smbd, ends the sessions of users that are gone or whose password changed, and with a domain empties winbindd's caches when the users changed. A state of the other mode is refused. The same state twice changes nothing |
| `acl` | sets the ACL of directories, their default ACL and that of the files directly in them, and gives each to `UNDERDOC_UID`; every path is opened below the tenant's directory without following links |
| `open` | the files open over SMB, by user, and whether for writing |
| `who` | the latest operations of smbd's audit that change something, which the helper receives as the syslog of its container, numbered, with the numbers of the oldest and the newest it keeps |
| `room` | the room of each library that a limit binds (path, free, total); the helper keeps one file per library in `<run>/room` (passed, not listed) for `dfree command`, and removes those not named. A companion from before answers it as unknown: UnderDoc says so once and clients see the disk |

No request carries a command line, and every name, UID, hash and path is checked against the rules of the channel
(`internal/samba`). A share path never holds `%`, which smb.conf would read as a variable such as `%U`: a folder with `%`
in its path is no share of its own and stays reachable through its library; `%` in a comment becomes a space.

## Connecting from Windows

In the Explorer, **Map network drive**, folder `\\<server>\Company` (or the share the profile lists), **Connect using
different credentials**, user name `<server>\<user name>` and the app password. Windows keeps it in its credential manager.
Without a domain, Windows signs in with NTLMv2, which Microsoft lists as deprecated but keeps working. With a domain, a
computer of the domain signs in by itself with Kerberos; elsewhere the user name is `<DOMAIN>\<user name>` with the
password of the domain.

## Limits

- ~~**Guessed names above a share of one's own.**~~ Closed on 2026-11-24 (T-43): the folders someone only passes
  through (`--x`) on the way to a share of their own are hidden from them in the share of the library (`veto files`, a
  list per user and share, `include = …/%U.conf` in the share; until a user's file is read, the share hides every such
  folder of anyone, so a missing file hides more, never less). A guessed name there answers
  `NT_STATUS_OBJECT_PATH_NOT_FOUND` whether it exists or not; the share of their own stays reachable as before
  (`TestSambaPassageTellsNoGuessedNames`).
- **A name like a readable one.** Samba hides by name, not by path: where the user reads a file or folder of the same
  name (in any case) in the library, the folder passed through is not hidden, as hiding it would take the readable one
  away too; that one name still tells a guess, as before T-43, and the log says so ("keeps telling a guessed name"). So
  does a name with `*`, `?`, `%`, `/` or `\`, which a veto list cannot hold (`TestSambaPassageNameLikeAReadableOne`).

## Tests

- `scripts/check-samba.sh` (in the required check of the CI, locally with Docker: `sg docker -c scripts/check-samba.sh`):
  builds the companion, starts one for the tenant main and one for acme, and runs the tests with the tag `samba`
  against them with `smbclient`: a save becomes a version, the save of Office through a rename keeps the ID, deleting
  goes to the trash, rights per folder hold, a user without a role sees nothing, a wrong app password is turned away, a
  removed app password stops at once, and a user of one tenant never reaches the other. The attack round of 2026-11-20
  (`attack_samba_test.go`) runs there too: a flood of listings, a hard link, a move with an old ACL, files that break the
  inheritance.
  For the domain (T-42) it starts a domain controller of Samba with made-up accounts and a password made for the run
  (never written into the repository), and a companion for the tenant corp that joins it. The script checks that the
  join happened, that a new container on the same volume starts without the password file, that one not joined and
  without it ends and says why, and that the password stands in no log of the companion and in none of its processes
  (command lines and environments). `samba_domain_test.go`: a domain account signs in by NTLMv2 and Kerberos and saves
  as its account, a number of the gate maps too; a domain account without a Samba user, `daemon`, the administrator of
  the domain, a wrong password and a local user of the companion are turned away; the companion refuses a state with
  app passwords; a UID that passes from one account to another is not reached by the first one's domain account.
- `scripts/t1-libreoffice.sh` (by hand, it mounts a share inside a container): LibreOffice saves over a real share while
  smbd's audit records every operation; the recordings in `internal/share/testdata/t1` are replayed by
  `TestRecordedSaves` in every run (T-1).
