---
id: ADR-10
type: Decision
title: "Ways to reach the files: SMB, WebDAV, browser"
status: erledigt
tags: [architecture, excel, webdav]
created: 2026-11-06
---

## Context

Windows users on a company network want a normal folder. Others have no SMB access: customers outside the network, macOS and Linux, mobile. SMB does not belong on the internet, and it cannot use OIDC.

## Options

- **SMB with a per-user password:** native on Windows, and the main path on a local network. Without a domain it relies on NTLMv2, which Microsoft lists as deprecated, although it keeps working for now.
- **SMB in a domain:** Kerberos and no extra sign-in, but needs the domain's approval.
- **WebDAV over HTTPS, served by the app:** works through HTTPS with the same permissions as the web, per user. The Windows WebClient service is deprecated and not started by default, and community sources name a default file limit of 50 MB, so it is not the Windows default. Clients such as rclone, or the macOS and Linux ones, are the target.
- **Check-out in the browser:** download, edit, upload with a hash check. Needs no client at all.
- **Sync client of our own:** comfortable, but a large product of its own.

## Decision

SMB with a per-user password first, and a domain join later where possible. WebDAV with per-user app passwords and browser check-out as further doors (M-6). No sync client of our own.

## Consequences

- All doors end in the same permission model, and the app is its source.
- WebDAV runs inside the app, so permissions can be finer than on SMB.
- Microsoft documents `ms-excel:ofe|u|` for https, so a WebDAV door could also enable an open-in-Excel button. To be tested.
