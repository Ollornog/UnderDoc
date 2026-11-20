# Samba

Files of the view **Files** as a network drive on Windows, macOS and Linux, over SMB ([ADR-30](../backlog/ADR-30-samba-share.md),
ADR-28, ADR-10). What is saved there becomes a version in UnderDoc, and what changes in the web appears on the share.
UnderDoc never runs as root: a **companion** container runs smbd, and UnderDoc steers it over a Unix socket.

## What is shared

- **Company** (`\\<server>\Company`): the folder of the tenant, once the tenant has a companion.
- **My files** (`\\<server>\<user name>`): the folder of each account that has an app password, through `[homes]`, for that
  account alone.
- **Collections** whose managers switched **Share over Samba** on in their rights dialog, each a share named after it.
- **A folder in which someone holds a role without reading the folder above** is a share of its own for them, named after
  the folder, so they reach it without seeing what lies above (ADR-29).

Only shared libraries are on disk; the content store keeps every version either way. A library that stops being shared
leaves the disk after a last pass has taken what changed there into the database; if something there is not taken (its
writer unknown, or refused), the directory stays on disk, the log says what, and every pass looks again.

## Rights on the share

Roles become POSIX ACLs per folder: read and comment read, edit and above write, no role sees nothing. `hide unreadable`
hides what a user may not read, and `access based share enum` lists only the shares a user reaches. **Samba knows rights
per folder only** ([T-1](../backlog/T-1-measure-office-saving.md)): a save through a temporary file and a rename makes a
new file that takes the rights of its folder, so a right on a single file would not survive it. In a shared library a
role granted on a single file holds in the web only, and the rights dialog says so; breaking the inheritance of a single
file is refused there, a folder can break it. For the same reason a collection holding a file that breaks the inheritance
is not shared, and nothing holding one moves into a shared library: both answer `409` naming the files, which must
inherit again first. A file that is in a shared library anyway (it broke the inheritance before the tenant had a
companion, or came back from the trash) is kept off the disk, and the log says so.

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
DELETE /api/samba/password              remove one's own
GET    /api/samba/accounts              administrators: every Samba user
DELETE /api/samba/accounts/<account>    administrators
PUT    /api/files/roots/<id>/samba      {"on": true}   managers of a collection of the files
```

The facts panel of a file shows its path on the share (`\\files\Company\Contracts\plan.xlsx`) and who has it open there;
`GET /api/files/<id>` answers it as `share`.

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
backports, the newest series a stable distribution carries, and the helper `underdoc-samba`. An example is
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
| `ping` | answers while smbd runs, with the versions and a name of this start: a companion that started again gets its state at the next pass |
| `apply` | the whole state of users (name, UID, NT hash, home) and shares (name, path, users); writes them, reloads smbd, and ends the sessions of users that are gone or whose password changed. The same state twice changes nothing |
| `acl` | sets the ACL of directories, their default ACL and that of the files directly in them, and gives each to `UNDERDOC_UID`; every path is opened below the tenant's directory without following links |
| `open` | the files open over SMB, by user, and whether for writing |
| `who` | the latest operations of smbd's audit that change something, which the helper receives as the syslog of its container, numbered, with the numbers of the oldest and the newest it keeps |

No request carries a command line, and every name, UID, hash and path is checked against the rules of the channel
(`internal/samba`). A share path never holds `%`, which smb.conf would read as a variable such as `%U`: a folder with `%`
in its path is no share of its own and stays reachable through its library; `%` in a comment becomes a space.

## Connecting from Windows

In the Explorer, **Map network drive**, folder `\\<server>\Company` (or the share the profile lists), **Connect using
different credentials**, user name `<server>\<user name>` and the app password. Windows keeps it in its credential manager.
Without a domain, Windows signs in with NTLMv2, which Microsoft lists as deprecated but keeps working; a domain comes
later (M-5).

## Limits

- **Guessed names above a share of one's own.** Whoever reaches a folder only through passage (`--x`) on the way to a
  share of their own cannot list the folders above it or read anything there, but can tell by the answer of the share
  (`NT_STATUS_ACCESS_DENIED` against `NT_STATUS_OBJECT_NAME_NOT_FOUND`) whether a name they guessed exists. Contents,
  listings and rights are not affected. Closing it would need a bind mount per share (`CAP_SYS_ADMIN` in the companion)
  or a module of smbd of its own, which is out of proportion; see ADR-30. Tested as a known limit in
  `TestSambaPassageKnownLimitGuessedNames`.

## Tests

- `scripts/check-samba.sh` (in the required check of the CI, locally with Docker: `sg docker -c scripts/check-samba.sh`):
  builds the companion, starts one for the tenant main and one for acme, and runs the tests with the tag `samba`
  against them with `smbclient`: a save becomes a version, the save of Office through a rename keeps the ID, deleting
  goes to the trash, rights per folder hold, a user without a role sees nothing, a wrong app password is turned away, a
  removed app password stops at once, and a user of one tenant never reaches the other. The attack round of 2026-11-20
  (`attack_samba_test.go`) runs there too: a flood of listings, a hard link, a move with an old ACL, files that break the
  inheritance.
- `scripts/t1-libreoffice.sh` (by hand, it mounts a share inside a container): LibreOffice saves over a real share while
  smbd's audit records every operation; the recordings in `internal/share/testdata/t1` are replayed by
  `TestRecordedSaves` in every run (T-1).
