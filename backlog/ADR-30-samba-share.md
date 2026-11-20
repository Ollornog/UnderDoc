---
id: ADR-30
type: Decision
title: "Samba: a working copy on disk, a watcher, and a companion that UnderDoc steers without root"
status: erledigt
tags: [files, samba, security, architecture]
created: 2026-11-20
---

## Context

ADR-28 settled the hybrid store: the content store keeps every version, and libraries shared over Samba also exist as real
files on disk. Its addendum names what Samba shows: the folder of the tenant as `\\server\Company`, the folder of each user
through `[homes]`, and each shared collection as a share of its own. ADR-29 settled who holds which role and set the limit
of a share: Samba knows rights per folder, per file only where T-1 proves that they survive a save. ADR-10 chose SMB with a
password per user, without a domain for now. M-5 asks that UnderDoc manages Samba users and rights without running as
root. The maintainer set the frame of this part on 2026-11-20: local users first, a domain later, real Excel is his
acceptance, passwords in the text (ADR-7) are a piece of their own.

This decision says how it is built, before the code.

## Options

**Where Samba runs and who steers it:**

- **smbd inside the image of UnderDoc:** one container, but UnderDoc would need root to switch users, and Samba's attack
  surface would sit next to the database credentials.
- **Samba on the host, steered through sudo:** no container, but root for UnderDoc through a back door, and every host
  looks different.
- **A companion container:** smbd and a small helper in a container of their own; UnderDoc talks to the helper over a Unix
  socket, and the helper accepts a few defined requests only. UnderDoc stays without root, and Samba sees only the files
  of its tenant.

**How rights reach the share:**

- **One Unix user for everyone** (`force user`) and the lists of a share (`valid users`, `read list`): simple, but rights
  hold per share only, never per folder.
- **One Unix user per Samba user, and POSIX ACLs per folder** that UnderDoc computes from the roles: the kernel enforces
  them, `hide unreadable` hides what one may not read, and a save through a temporary file and a rename keeps the rights of
  its folder by inheritance.
- **A VFS module of our own:** exact, but C inside smbd, maintained by us.

**How a change on the share reaches the database:**

- **Poll the tree:** simple and works on any file system, but slow and costly on large trees.
- **inotify:** events per directory, works without root and across containers on the same file system; a full scan at
  start and after an overflow catches what it missed.
- **fanotify:** fewer watches, but needs `CAP_SYS_ADMIN` for the useful modes.

## Decision

A companion container with smbd and a helper, Samba users as Unix users of that container with POSIX ACLs per folder,
and inotify with a full scan as the safety net. In detail:

### The pieces

- **UnderDoc** (no root, its own UID) owns the working copy: it writes the files of shared libraries to disk, watches them
  and takes every change into the database. It computes users, shares and ACLs from the database and hands them to the
  helper.
- **The companion** (image `underdoc-samba`, built from this repository) runs smbd and the helper. The helper is its first
  process: it starts and supervises smbd, receives Samba's audit through a syslog socket of its own, and answers on a Unix
  socket. It runs as root inside its container, because smbd must switch to the UID of each user; it has no network of its
  own besides port 445 and no access to the database.
- **Samba itself** is the newest series a stable distribution carries, 4.24 from Debian 13 backports when this was
  written; the image pins base and package and moves with them.
- **One companion per tenant.** It mounts only the directory of its tenant, at the same path as UnderDoc sees it, so the
  kernel keeps tenants apart even if everything above failed. A user of tenant A is never a user of the companion of
  tenant B. With many tenants WebDAV is the door (ADR-28, M-7); a companion per tenant is for the few that want SMB.

### The channel

- A Unix socket in a directory both containers mount. The helper creates it with mode 0600 for the UID of UnderDoc, and
  checks the peer of every connection (`SO_PEERCRED`): another UID is turned away.
- One JSON request per connection, from a closed list:
  - `ping`: the helper and smbd are up, with their versions.
  - `apply`: the whole wanted state of users and shares. The helper makes it so and only then reloads smbd: users it does
    not know are added (Unix user, Samba account with the NT hash given), users that are gone are removed and their
    sessions closed, a changed password closes the user's sessions, the shares are written into an included file.
    Applying the same state twice changes nothing.
  - `acl`: ACLs for directories and the files directly in them, each path below the tenant's directory, opened without
    following links; the helper also gives each such path to the UID of UnderDoc.
  - `open`: the files open over SMB right now, with user and whether for writing (`smbstatus`).
  - `who`: who wrote, created, renamed or deleted a path lately, from the audit (`vfs_full_audit` to the helper's syslog
    socket).
- No request takes a command line, a path outside the tenant's directory, or a name the helper did not check. Names of
  Samba users are lower case letters, digits, `.`, `_` and `-`, at most 32, and never a user of the container's own
  (`root`, `nobody`, …).

### Samba users and the app password

- A Samba user is an account of the tenant that has made an **app password** in its profile; its folder in Files is
  shared to it through `[homes]` from then on. The name is its login when that fits the rules above, otherwise one it
  chooses; the name is fixed per tenant.
- The app password is made by UnderDoc (random, about 100 bits, in groups to type), shown once, and never stored. What is
  stored is the NT hash Samba needs for NTLMv2, sealed in the vault (`UNDERDOC_SECRET_KEY`, ADR-23), so the companion keeps
  no state of its own: a new companion gets every user back from UnderDoc. Samba needs no vault key and no password ever
  crosses the channel in plain text. Without a vault key there is no Samba. The helper writes the hashes into the
  password file of the companion (the `smbpasswd` back end of Samba), so no command line ever carries one.
- A new app password replaces the old one and closes its sessions; deleting it removes the Samba user. An account the gate
  or the identity provider no longer lets in keeps its app password until an administrator or the account removes it,
  since UnderDoc deactivates no account (ADR-29); administrators see and remove app passwords in the tab Rights.
- The NT hash is equivalent to the password for NTLM, so it is sealed like the client secret of OIDC, and the log names the
  making and removing of an app password, never a value.
- smbd allows SMB 2.02 and later only, NTLMv2 only, no guest, no SMB1, no POSIX extensions (no links, no modes from the
  client), no extended attributes from the client (`ea support = no`), and signs where the client offers it. Port 445 never
  faces the internet (ADR-28): the example binds it to an address of the LAN or the mesh.

### The working copy

- **Only shared libraries are on disk:** the folder of the tenant (`Company`) once Samba is set up for the tenant, the
  folder of each account that has an app password (`[homes]`), and each collection whose manager switched its share on. A
  library that stops being shared leaves the disk; the store keeps everything.
- **Layout:** `<share dir>/<tenant>/<ID of the root>/…` with the real names of folders and files. The ID of the root, not
  its title, names the directory, so renaming a collection renames its share only.
- **The ID sticks to the file** as the extended attribute `user.underdoc.id`, on files and folders. Clients cannot write
  extended attributes (`ea support = no`), and a forged ID gains nothing anyway: every change is taken as the user who made
  it, through the same functions of the database as the web, so it can do only what that user may.
- **Every change on the share becomes a change in the database** after a quiet moment (two seconds without an event on the
  path, and the file no longer open for writing):
  - a changed file becomes a new version, with the user who wrote it as its author;
  - a new file or folder becomes an object in the folder above it, owned by the user who made it;
  - a file whose ID moved to another path was renamed or moved;
  - a file without an ID at a path where the database knows a file is a new version of that file, which then gets the ID:
    this is the save through a temporary file and a rename of Office (T-1). A file with an ID of its own that is renamed
    over a known file replaces it, and the replaced one goes to the trash;
  - a file or folder that is gone is moved to the trash (ADR-22), as the user who deleted it; its versions stay.
- **Temporary files of Office and of the systems never become objects:** `~$*`, `~*.tmp`, `*.tmp`, `.~lock.*#`,
  `lu*.tmp`, `Thumbs.db`, `desktop.ini`, `.DS_Store`, `._*`, the names smbd itself uses while it makes a folder
  (`.::TMPNAME:*`), and the temporary files of UnderDoc itself, which smbd also hides (`veto files`). The list is backed
  by the recordings of T-1.
- **Who made a change** comes from the audit of the companion. Without one (the helper unreachable), the owner of the new
  file on disk names it; failing that, the writer is unknown, and the change is put back (settled after the attack round
  below; until 2026-11-20 it was taken as the owner of the object).
- **A change in the web writes to disk:** a new version, a rename, a move, a deletion, a restore, a new folder. UnderDoc
  writes a temporary file next to the target and renames it, so a reader never sees half a file, and the watcher takes
  its own write for what it is: the file on disk equals the database, and nothing happens.
- **Locked while open:** while a file is open over SMB, the web does not write it. Uploading a new version, restoring one,
  renaming, moving or deleting it answers that it is open, and by whom; the facts panel of the file says so too.
- **What the database refuses goes back:** a change the user may not make, a file larger than allowed, or a quota that is
  full, is not taken over. A refused rename, move or deletion is put back on disk from the database; a refused new content
  stays on disk, is not versioned, and the problems of the files name it, until a later change can be taken.
- **A table of what was in step** (`share_files`: path, inode, size, time and hash per object) tells after a restart which
  side changed: a file that differs from its row changed on disk and becomes a version; an object whose current version
  differs from its row changed in the web and is written. If both changed, the disk becomes a new version on top, so
  nothing is lost. A full scan runs at start, after an overflow of inotify, and once an hour.

### Rights on the share

- **Roles to ACLs, per folder:** read and comment give `r-x` on a folder and `r--` on its files, edit and above give `rwx`
  and `rw-`, no role gives nothing. UnderDoc's UID has `rwx` everywhere; owner, group and others have nothing. Directories
  carry the same as default ACL, so what Office or anyone makes inherits the rights of its folder.
- **Who reaches a share:** the users who read the root of its library. `access based share enum` lists only the shares a
  user reaches, and `hide unreadable` hides folders and files a user may not read.
- **A folder in which a user holds a role without reading the folder above it becomes a share of its own** for that user,
  named after the folder (ADR-29: "a folder can break it, and becomes a folder of its own for Samba"). The folders above
  give that user passage only (`--x`), so smbd reaches the share without the user seeing or listing anything above it.
- **Within a shared library, rights hold per folder** until T-1 proves more (ADR-29): a role granted on a single file holds
  in the web only, and the rights dialog says "Samba knows rights per folder only"; breaking the inheritance of a single
  file is refused there. A folder can break it.
- Every change of a role, of the inheritance, of an owner, of a group or its members, and every new or removed app password
  computes the ACLs of the libraries concerned again; the hourly scan does it for all.
- **The private folder of a user** is a share for that user alone, through `[homes]`.

### Shown in the web

- The facts panel of a file and a folder in a shared library shows its path on the share, `\\<server>\<share>\<path>`, to
  copy; the server is the name of the instance's setting `UNDERDOC_SAMBA_SERVER`.
- The profile has the app password for SMB: the user name, making a new one (shown once), removing it, and the paths of the
  shares the account reaches.
- The rights dialog of a collection switches its share on and off, for its managers, and says per folder and file what
  Samba shows.

### Settled after an attack round (2026-11-20)

An independent attack round against real companions found ways around the rules above; each has a test that was red
before (`internal/share/attack_samba_test.go`, `attack_test.go`, `hardening_test.go`).

- **An unknown writer is nobody.** An editor who moved a file (which needs manage, ADR-29) and then pushed the rename out
  of the audit's ring with listings had the move taken as the file's owner. Now a change whose writer neither the audit
  nor a Samba user owning the file names is never taken as anyone: a move, a rename, a deletion and a save are put back
  from the database, something new stays on disk untaken, and the log says each. The audit keeps only operations that
  change something (a `create_file` that opens without asking to change is dropped), 16384 of them, numbered; a pass that
  finds a gap since the one before logs it. A change made before an app password was removed is still taken as its
  account, which UnderDoc remembers while it runs.
- **Hard links end at the next pass.** SMB2 makes them, Samba has no setting against them, and two names shared one ID.
  The name the record of the ID knows keeps the object; every other name becomes a copy without an ID, a new file of whoever
  made the link (`linkat` is audited now). A link from another library never takes the ID either.
- **A file gets the ACL of its folder whenever it comes to a new path or a new inode.** The companion set the ACL of a
  folder's files only when the folder's own changed, so a file moved into a narrower folder kept the readers of the one it
  left, and a file made over SMB kept its maker as owner.
- **A file that breaks the inheritance never reaches the share.** Only breaking it inside a shared library was refused.
  Now sharing a collection that holds one and moving one into a shared library are refused too, naming the files; one that
  is in a shared library anyway (the folder of the tenant shared at setup, an app password that shares a user's folder, a
  restore from the trash) stays off the disk, and the log says so.
- **A library that stops being shared gets a last pass first.** It left the disk with whatever was saved since the pass
  before; now that is taken into the database first, and a directory still holding something untaken stays on disk.
- **A Samba user never takes the name of a share.** The web refuses such a name, and a share keeps its name from the pass
  before, so mapped drives do not break.
- **The patterns of temporary files hold for files only.** A folder named `x.tmp`, `._x` or `~$x` kept a whole tree out of
  the database, the quota and the versions; it is an ordinary folder now. A file of the web named like a temporary one
  stays off the disk, where the scan would have taken it for deleted.
- **No `%` in a share path:** smb.conf reads it as a variable (`%U` is the user's name), so a folder with `%` in its path is
  no share of its own, and the helper refuses such a path; in a comment it becomes a space.
- **Known limit: guessed names above a share of one's own.** Passage (`--x`) through the folders above a share of one's
  own lets its user tell an existing name there from a missing one (`NT_STATUS_ACCESS_DENIED` against
  `NT_STATUS_OBJECT_NAME_NOT_FOUND`): a guessed name is confirmed, nothing more. Decided on 2026-11-20 not to fix it:
  closing it needs the share's folder outside the tree (a bind mount per share, which the companion may not make without
  `CAP_SYS_ADMIN`, a capability worth more than this) or a module of smbd of its own, both out of proportion to a leak
  of names that must be guessed one by one. What stays protected: contents (reading fails), listing (the passage
  folder cannot be listed and shows no name) and rights (passage grants nothing else). The test
  `TestSambaPassageKnownLimitGuessedNames` holds both: the listing and the read stay closed, and the guess is named as
  the limit; if the guess ever closes, that test goes red and this entry and `docs/samba.md` are to change.

- **The quota reaches the client (decided on 2026-11-20).** Until then a save over SMB past a quota only stayed on disk
  without a version and was named in the log: Windows and Excel believed it saved. Samba has no quota of its own, and
  the quotas of the file system need root and a quota per user, not per library and tenant as UnderDoc counts them
  (ADR-28). Of what Samba offers, `dfree command` is the one that fits: smbd asks a program how much room a directory
  has, and tells the client. So:
  - After each pass UnderDoc tells the companion the room of each shared library that a limit binds: the quota of the
    library (or of a user's own folder), the limit of the tenant, the storage the hoster allows, whichever leaves the
    least; versions count, as they do in the web. A new request of the helper, `room`, takes the whole list; the helper
    keeps one small file per library in a directory of its own (readable by name only), and `underdoc-samba dfree`,
    which smbd runs for `dfree command`, answers from the nearest library above the directory asked, never more than
    the disk has. A library without a limit answers what the disk has, as before. `dfree cache time` is 5 seconds.
  - **What this does:** Windows shows the room of the library on the drive, Explorer refuses a copy that does not fit
    before it starts, and an application that asks for room before it saves fails there. **What it does not:** smbd
    does not refuse a write itself, so a save that does not ask first still lands on disk; it stays without a version
    as before. A single file larger than allowed (`UNDERDOC_FILES_MAX_SIZE`) cannot be told by room at all.
  - **The person who saved learns it in the web:** each save or new file over SMB that a quota or the largest size
    refuses is logged once with their account as actor (their part of the log), and the workspace says so when they open
    it (a notice with the file and why), and lists it in the profile under Samba, until the file is taken or gone. It is
    kept in memory and found again by the next pass after a start.
  - A companion from before answers `room` with an error: UnderDoc says so once in its log and goes on without it.
  - **Built on 2026-11-20** ([docs/samba.md](../docs/samba.md)). smbd runs `dfree command` as one path with the
    directory as its only argument and does not split the setting into words, so it names a link `dfree` to the helper
    in the helper's directory (the first try, the helper with an argument, ran nothing, and smbd fell back to a made-up
    room of 2 MB). Building it showed that a refused save did not stay on disk: the same pass wrote the database's
    content over it, so the save was lost; now nothing is written over a refused save, and each pass takes it up again
    until there is room or it is gone. A real smbd shows the room in `TestSambaQuotaReachesTheClient`.

### Not now

- **A domain** (M-5, ADR-10): Kerberos instead of NTLMv2, later, next to local users.
- **Rights per single file on the share:** only if T-1 shows they survive a save of Excel and LibreOffice.
- **Passwords in the text** (ADR-7): a piece of their own.

## Consequences

- UnderDoc needs the share directory (`UNDERDOC_SHARE_DIR`) on a file system with extended attributes and POSIX ACLs, such
  as ext4, XFS or btrfs, mounted in the companion at the same path; NFS and other network file systems are not supported,
  since inotify does not see changes made elsewhere.
- The UIDs of Samba users come from a range that fits an unprivileged container (20000 to 29999 by default), so the files
  stay valid inside an LXC as well.
- A backup still takes the database and the content store (ADR-28); the working copy is written again from them.
- Tests start a real companion in a container: saving, renaming, deleting, rights per folder, a user without a role, a wrong
  app password and two tenants run against smbd itself (T-1, M-2, M-5).
- **Measured on 2026-11-20 (T-1, LibreOffice 26.8 over Samba 4.24):** LibreOffice saves into `lu<random>.tmp` and renames
  it over the file; the saved file is a new inode that takes the ACL of its folder, so a right on a single file does not
  survive a save, and rights stay per folder. A file without read access stays hidden. The watcher takes the save as a
  version of the same object; the recordings are fixtures of its tests. Real Excel stays the maintainer's acceptance.
