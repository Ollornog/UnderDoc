# Accounts

People sign in through the gate in front of UnderDoc, TinySesam ([sign-in.md](sign-in.md),
[ADR-32](../backlog/ADR-32-sign-in-only-through-the-gate.md)). The gate keeps the accounts; UnderDoc keeps none of a
person's credentials. Since T-33 UnderDoc asks the gate itself, through the gate's **account API**, to invite a
colleague, to give a new sign-in link, to lock and unlock an account and to reset its access, and it has deleting for
good confirmed by the gate's **approvals** ([ADR-35](../backlog/ADR-35-what-the-gate-decides-and-what-the-app-decides.md)).

All of it is **optional**: without the settings below nothing is offered, the buttons are not there, and an instance
behind a gate that has no account API for it works as before. What holds without them as well: the lock of UnderDoc
(below), the delegated administrator, and `underdoc account add`.

- [Who may do what](#who-may-do-what)
- [Inviting](#inviting)
- [A new sign-in link, locking, resetting access](#a-new-sign-in-link-locking-resetting-access)
- [The lock of UnderDoc](#the-lock-of-underdoc)
- [Two levels of administration](#two-levels-of-administration)
- [Deleting for good with an approval](#deleting-for-good-with-an-approval)
- [Accounts that exist already](#accounts-that-exist-already)
- [Configuration](#configuration)
- [The log](#the-log)

## Who may do what

The gate takes the person UnderDoc names as the one it acts for on trust (`TinySesam-On-Behalf-Of`); it checks only
UnderDoc's service account, its scopes, its allowlist of roles and its limits. So **UnderDoc checks itself** who may do
what, before it calls the gate:

| Who | Invites | New link, lock, unlock | Resets access |
|---|---|---|---|
| An **administrator** (`UNDERDOC_ADMIN_ROLE`) | into every local group | every account but their own | every account, as the gate's approver |
| A **delegated administrator** (`UNDERDOC_DELEGATE_ROLE`) | into every local group | every account but an administrator's, another delegate's or their own | no |
| A **group admin** | into the groups they manage | the persons directly in a group they manage, or in a group inside it | no |
| Anyone else | no | no | no |

- **Only a person in the browser**, signed in as a session (`Remote-Auth: session`): never an agent, a key, an app
  password, OAuth, MCP or a service account, whatever roles it holds (ADR-36, B-4). MCP has no tool for any of it.
- **Never a service account** as the account acted on, and never one's own account.
- **A group** taken over from the gate, or an archived group, takes no invitation.
- **Roles:** an invitation gives only roles of UnderDoc's allowlist (`UNDERDOC_ACCOUNTS_ROLES`), never a role of an
  administrator, of the delegated administrator, of who creates spaces, or the gate's approver role; the start refuses
  such a list. The gate holds its own allowlist and refuses anything off it as well.

## Inviting

A group admin opens a group in the tab **Rights**, **Members**, and invites a colleague with a login, a name and a mail
address if wanted. UnderDoc asks the gate to make the account (`POST /auth/api/users`, with UnderDoc's roles), and the gate
answers with its lasting ID and a one-time sign-in link. UnderDoc **binds a profile to that ID at once** and puts it into
the group, so whatever is shared with the group is there at the first sign-in. The link is shown **once**, to hand over;
UnderDoc keeps it nowhere and logs it nowhere. A profile is only ever bound by the lasting ID of the gate: an invitation
under the login of someone in UnderDoc makes another account, it never takes over theirs.

The person follows the link, sets up how they sign in on the gate's page, and comes into the profile prepared for them.

## A new sign-in link, locking, resetting access

**Account** beside a person in a group, and among the users for administrators and delegates, shows the account in
UnderDoc and in the gate, with what the viewer may do:

- **New sign-in link:** only before the first sign-in, and not while the account is locked; older links die. After the
  first sign-in a link would be a takeover, and the gate refuses it.
- **Lock** and **Unlock:** in the gate, where it lets UnderDoc (it manages only accounts UnderDoc made or was handed,
  [below](#accounts-that-exist-already)), and **always in UnderDoc** ([the lock of UnderDoc](#the-lock-of-underdoc)). An
  account the gate's operator locked is unlocked only by the operator.
- **Reset access** (new phone, forgotten password), after the first sign-in: UnderDoc asks the gate for its approval
  `tinysesam.account.reset_access`, and the person who asked **confirms it in the gate** with the code shown and a step-up
  of their own. The gate lets only a holder of its approver role confirm it, which ADR-35 sets to UnderDoc's administrator
  role, so only administrators are offered it; that role is never on the allowlist. Then the password, the factors, the
  keys and the sessions of the account go, and UnderDoc picks the new sign-in link up **once** and shows it to whoever
  asked.

## The lock of UnderDoc

A locked account comes in through **no way**: the workspace, the API, MCP and its upload links answer *Locked* (403), its
Samba user is taken off the share, and the one check of the rights refuses it every action (`locked` as the deciding
entry), so neither its own objects, nor entries, nor links it made or took give anything: a share link it made answers
like one that never was while it is locked. Nothing of it is lost; unlocking brings all of it back. The lock holds on its
own, also where the gate does not lock (an account the gate does not let UnderDoc manage, a gate that did not answer) and
until a gate token of the account expires. In the database: `accounts.locked`, `locked_by` (migration 040).

## Two levels of administration

- The **administrator** holds the role of the app `UNDERDOC_ADMIN_ROLE` (`underdoc-admin`), or the mark of the command
  line ([sign-in.md](sign-in.md#the-emergency-access)). They do everything below and more.
- The **delegated administrator** holds the role of the app `UNDERDOC_DELEGATE_ROLE` (`underdoc-delegate`), its own:
  - manages the accounts of every group: invites into every local group, gives new sign-in links, locks and unlocks;
  - sees the users and the members of every group;
  - archives and restores groups (B-5).

  Not: modules, how long the log keeps entries, the rules page, the marks of administrators, the rights of objects
  beyond their own, the accounts of administrators and delegates, resetting access.
- A service account is neither, whatever roles the gate gives it, and an agent of either holds none of it (B-4).

The permissions are decided in one place (`account.User.Has` with `PermAccounts`, `PermArchiveGroups`,
`PermAdminister`), so that they can move to the gate's own permissions later (TinySesam's planned permission layer)
without the rest of UnderDoc changing.

## Deleting for good with an approval

With `UNDERDOC_APPROVALS=on`, deleting something for good from the trash and emptying the trash also need a **signed
one-time approval of the gate** (B-3): the gate's Go client checks it (`VerifyApproval`), its id (`jti`) is used once
(table `approval_jti`).

1. In the trash, **Delete for good** with the name typed in (or **Empty the trash** with `Trash`). UnderDoc checks
   everything first: owner or administrator, browser session, the name, everything below may go.
2. Only then UnderDoc asks the gate (`<app>.object.purge` with the object and its title, or `<app>.trash.empty` with the
   number of items and a **cutoff**) and sends the browser to the gate's page. The person compares the code and confirms
   with a step-up there.
3. The gate sends the browser back to `/approvals/return`. Only the browser that asked comes back (a cookie with the
   state, compared with the state the gate returns and the one UnderDoc kept), only the account that asked, once.
4. UnderDoc picks the confirmation up, locks what goes, **checks again**, then checks the confirmation (this action, these
   fields, this person, this browser, once) and deletes. A purge the check refuses on the way back uses no confirmation;
   emptying the trash takes only what was deleted at or before its cutoff, never what came after (B-6).

Without the setting, deleting for good stays as before: owner or administrator, in the browser, with the name typed in.

## Accounts that exist already

The gate's account API shows UnderDoc only the accounts it made itself. Accounts that exist before are handed to UnderDoc
by the **gate's operator** (`tinysesam accounts adopt --app underdoc …`, or *Take over for …* in the gate's panel). Then:

```bash
underdoc account [--tenant <id>] adopt [--dry-run]                # an account for each the gate shows UnderDoc, by its ID
underdoc account [--tenant <id>] add <remote-id> <login> <name>   # one account bound to a lasting ID, for an import
underdoc account [--tenant <id>] list                             # every account, its ID, its source, its mark
```

Both bind by the lasting ID alone and give no mark of an administrator; an ID bound already keeps its account. An account
made before ADR-32 is bound with `underdoc account bind` ([sign-in.md](sign-in.md#accounts-made-before)).

## Configuration

| Setting | Meaning | Default |
|---|---|---|
| `UNDERDOC_GATE_URL` | The gate's own address, its `base_url`: the calls go there directly, not through the proxy of UnderDoc. https, plain http only to the own machine | none |
| `UNDERDOC_GATE_SERVICE_KEY_FILE` | A file that holds the automation key of UnderDoc's service account in the gate, and nothing else (mode 0600). Never the key itself in the environment, and never in a log | none |
| `UNDERDOC_GATE_APP` | The id of UnderDoc in the gate's `apps` | `underdoc` |
| `UNDERDOC_ACCOUNTS_ROLES` | The roles UnderDoc gives a person it invites, separated by commas, a part of the gate's allowlist. Set, it turns the accounts on | none: off |
| `UNDERDOC_APPROVALS` | `on`: deleting for good waits for an approval of the gate | off |
| `UNDERDOC_DELEGATE_ROLE` | The role of the app that makes a delegated administrator ([sign-in.md](sign-in.md#the-gate)) | `underdoc-delegate` |

**At the start** UnderDoc asks the gate what its key may do (`GET /auth/api/provisioning`) and does not start when the
app is another, a scope is missing (`users:read`, `users:invite`, `users:disable`; `users:reset` turns the reset on), a
role of `UNDERDOC_ACCOUNTS_ROLES` is not on the gate's allowlist, or one of them is the approver role or a role of
administrators. The accounts and the approvals need the database.

**In the gate**, the operator configures the app as the gate's documentation says (*Accounts for applications: the
account API*, *Approvals for application actions*), for example:

```python
apps={"underdoc": {
    "account": "svc-underdoc",                       # a service account with an automation key
    "name": {"en": "UnderDoc", "de": "UnderDoc"},
    "provisioning": {
        "roles": ["underdoc-user"],                  # UnderDoc's allowlist, never an administrator's role
        "scopes": ["users:read", "users:invite", "users:update", "users:disable", "users:reset"],
        "approver_role": "underdoc-admin",           # who confirms a reset: UnderDoc's administrators (ADR-35)
    },
    "approvals": {
        "return_urls": ["https://wiki.example.com/approvals/return"],
        "types": {
            "underdoc.object.purge": {"fields": {"object": {"type": "string", "max": 40},
                                                 "title": {"type": "string", "max": 200}},
                                      "title": {"en": "Delete for good"},
                                      "summary": {"en": "Delete {title} ({object}) and everything below it for good."},
                                      "irreversible": True},
            "underdoc.trash.empty": {"fields": {"items": {"type": "integer", "min": 0, "max": 1000000},
                                                "cutoff": {"type": "integer", "min": 0, "max": 9007199254740991}},
                                     "title": {"en": "Empty the trash"},
                                     "summary": {"en": "Delete {items} items of your trash for good."},
                                     "irreversible": True},
        },
    },
}}
```

UnderDoc itself speaks only HTTP to the gate and uses the gate's Go client, never TinySesam's Python API. The test of
this repository runs exactly such a configuration against a real TinySesam
([`scripts/gate/gate.py`](../scripts/gate/gate.py), in `scripts/check-services.sh`). Until the gate's ready-made gateway
reads `apps` from its own configuration (TinySesam T-97, a configuration file in TOML), a gate with the account API is
TinySesam as a library with such a configuration. Later TinySesam is meant to run as a companion of UnderDoc in its
Compose file, with local accounts and OIDC side by side; the HTTP calls stay the same.

## The log

Every one of these is a line of the log (ADR-11), with the lasting ID UnderDoc named to the gate as the one it acted for:
`invited`, `sign-in link`, `locked`, `unlocked`, `access reset`, `account added` (the command line and adopt), and
`deleted for good` with *confirmed in the gate*. A sign-in link is never in it.
