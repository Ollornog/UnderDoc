// SPDX-License-Identifier: MIT

// What UnderDoc pages really hold: every editor starts with exactly this text, and its round trip is compared with it.
export const fixture = `# Network plan

The **router** sits in the *rack* and runs \`nftables\`. See [[Wiki links]], [the part](/o/7k2m9q4x1c#h-part) and [the vendor](https://example.com/docs).

## Devices

- Router
- Switch with **PoE**
  - Patch panel

1. Unpack
2. Mount
3. Connect

- [ ] Order cables
- [x] Label ports

| Name | Value |
| --- | ---: |
| Height | 50 m |
| Ports | 24 |

\`\`\`text
router 192.0.2.1
\`\`\`

> A quote with *emphasis*.

::embed{ref="7k2m9q4x1c#b2"}

---

![alt](/o/x.png)
`;
