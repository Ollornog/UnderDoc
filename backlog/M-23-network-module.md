---
id: M-23
type: Milestone
title: IP addresses and devices
status: offen
blocked_by: [M-3, M-5]
tags: [module]
created: 2026-11-07
---

Network documentation as document types an administrator can switch off, with ideas from tools such as NetBox. Only an idea so far:
the talk before this milestone (ADR-14) settles the scope.

**To settle in that talk:**

- A small module of its own, or an existing NetBox connected as a source (M-4). Using a finished service comes first if it fits.
- Which objects are needed: IP ranges, addresses, VLANs, devices, ports, racks.

**Done when this is true:**

- IP ranges and device lists are document types, so they sit in the workspace like any other document, with roles, tags and children.
- Pages embed a range or a device, and a value (M-3) can come from an address, so a changed address shows everywhere.
- It documents what should be. It does not scan the network.
- Switched off, the types disappear, and no data is touched.
