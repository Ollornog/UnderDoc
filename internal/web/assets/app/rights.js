// SPDX-License-Identifier: MIT
// Rights (ADR-29, docs/rights.md): the tab Rights of nav1 and the rights dialog of a document, folder or file. The server
// decides who may do what; this script only shows it and sends what a person changes. The workspace (demo.js) draws the
// tab through UDRights.nav3 and opens the dialog through UDRights.open; this script keeps what it loaded and asks the
// workspace to draw again when something arrives.
window.UDRights = (() => {
  const ROLES = [["read", "Read"], ["comment", "Comment"], ["edit", "Edit"], ["manage", "Manage"]];
  const ROLEWORD = { read: "read", comment: "comment", edit: "edit", manage: "manage", owner: "owner" };
  const MODES = [["read", "Read"], ["comment", "Comment"], ["edit", "Edit"], ["upload", "Upload only"]];
  const VIA = { owner: "owns it", "owner above": "owns what lies above", person: "as a person", group: "through a group", tenant: "as everyone", link: "through a link" };
  const st = { people: null, groups: null, links: null, users: null, log: null, keep: null, held: null, heldFor: "", check: null, who: null, members: {}, open: {}, error: "", busy: false };
  let redraw = () => {};
  let me = { id: "", admin: false };

  const esc = (s) => String(s ?? "").replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c]);
  const when = (s) => (s ? new Date(s).toLocaleString() : "");
  const day = (s) => (s ? new Date(s).toLocaleDateString() : "never");

  async function api(method, url, body) {
    const init = { method, credentials: "same-origin", headers: { Accept: "application/json" } };
    if (body !== undefined) {
      init.headers["Content-Type"] = "application/json";
      init.body = JSON.stringify(body);
    }
    const r = await fetch(url, init);
    if (!r.ok) {
      let message = String(r.status);
      try {
        message = (await r.json()).error || message;
      } catch (e) {
        /* no JSON */
      }
      throw new Error(message);
    }
    return r.status === 204 ? null : r.json();
  }

  // load fetches what a section shows, once, and draws again when it is there.
  async function load(key, url, after) {
    if (st[key] !== null || st.open[key]) return;
    st.open[key] = true;
    try {
      st[key] = await api("GET", url);
      if (after) after();
    } catch (e) {
      st.error = e.message;
    }
    st.open[key] = false;
    redraw();
  }
  const reload = (...keys) => {
    keys.forEach((k) => {
      st[k] = null;
    });
    redraw();
  };

  const roleSelect = (name, current, disabled) =>
    `<select class="select" name="${name}" data-rsel="${name}"${disabled ? " disabled" : ""} aria-label="Role">${ROLES.map((r) => `<option value="${r[0]}"${r[0] === current ? " selected" : ""}>${r[1]}</option>`).join("")}</select>`;
  const whoSelect = (name, withTenant) => {
    const p = st.people || { people: [], groups: [] };
    return `<select class="select" name="${name}" aria-label="Who">${withTenant ? '<option value="tenant:">Everyone in this workspace</option>' : ""}${p.groups.length ? `<optgroup label="Groups">${p.groups.map((g) => `<option value="group:${esc(g.id)}">${esc(g.name)}${g.source === "synced" ? " (sign-in)" : ""}</option>`).join("")}</optgroup>` : ""}<optgroup label="People">${p.people.map((x) => `<option value="person:${esc(x.id)}">${esc(x.name)} (${esc(x.login)})</option>`).join("")}</optgroup></select>`;
  };
  // personSelect chooses an account. With choose, nobody is chosen until someone is, and without is left out: a form that
  // gives something away must not have a default that sends it somewhere by itself.
  const personSelect = (name, current, choose, without) => {
    const p = st.people || { people: [] };
    const first = choose ? '<option value="" selected disabled>Choose someone</option>' : "";
    return `<select class="select" name="${name}" aria-label="Person"${choose ? " required" : ""}>${first}${p.people.filter((x) => x.id !== without).map((x) => `<option value="${esc(x.id)}"${x.id === current ? " selected" : ""}>${esc(x.name)} (${esc(x.login)})</option>`).join("")}</select>`;
  };
  // small is a small outline button of C22: xs in the panel of the workspace, sm in the dialog.
  const small = (attrs, label, inDialog) => `<button class="btn" data-variant="outline" data-size="${inDialog ? "sm" : "xs"}" type="button" ${attrs}>${label}</button>`;
  const submit = (label) => `<button class="btn" data-variant="outline" data-size="xs" type="submit">${label}</button>`;
  const problem = () => (st.error ? `<p class="text-sm" role="alert" data-rights-error>✗ ${esc(st.error)}</p>` : "");
  // The parts of the panel in nav3, from C22: a heading, a quiet hint, entries of a list (an .item with a quiet line and
  // its buttons at the end).
  const head = (t) => `<h3 class="font-semibold">${t}</h3>`;
  const hint = (t, attrs = "") => `<p class="text-muted-foreground text-xs"${attrs}>${t}</p>`;
  const entry = (attrs, name, sub, side) =>
    `<div class="item" data-variant="outline" data-size="sm" role="listitem" ${attrs}><section><h4>${name}</h4>${sub ? `<p class="text-muted-foreground text-xs">${sub}</p>` : ""}</section>${side ? `<aside class="flex flex-wrap gap-1">${side}</aside>` : ""}</div>`;
  const entries = (attrs, inner) => `<div class="item-group" role="list" ${attrs}>${inner}</div>`;

  // --- The tab Rights

  const SECTIONS = [["groups", "Groups", "user"], ["users", "Users", "user"], ["links", "Share links", "link"], ["check", "Access check", "search"], ["log", "Log", "list"]];

  function nav3(sec, user) {
    me = user || me;
    load("people", "/api/people");
    let body = "";
    if (sec === "groups") body = groupsHtml();
    else if (sec === "users") body = usersHtml();
    else if (sec === "links") body = linksHtml();
    else if (sec === "log") body = logHtml();
    else body = checkHtml();
    return `<div><aside class="card gap-3 px-4 py-4" data-rights="${esc(sec)}">${problem()}${body}</aside></div>`;
  }

  function groupsHtml() {
    load("groups", "/api/groups");
    if (st.groups === null) return hint("Loading…");
    const rows = st.groups.map((g) => {
      const open = st.members[g.id];
      const local = g.source === "local";
      const list = open ? open.map((m) => entry("", esc(m.name), esc(m.login), me.admin && local ? small(`data-ract="unmember" data-g="${esc(g.id)}" data-id="${esc(m.id)}"`, "Take out") : "")).join("") : "";
      const add = open && me.admin && local ? `<form class="flex items-center gap-2" data-rform="member" data-g="${esc(g.id)}">${personSelect("account", "")}${submit("Add")}</form>` : "";
      return entry(`data-group="${esc(g.name)}"`, esc(g.name), `${g.members} ${g.members === 1 ? "member" : "members"} · ${local ? "local" : "from the sign-in, read only"}${g.description ? " · " + esc(g.description) : ""}`,
        small(`data-ract="members" data-g="${esc(g.id)}" aria-expanded="${!!open}"`, "Members") + (me.admin && local ? small(`data-ract="delgroup" data-g="${esc(g.id)}" data-name="${esc(g.name)}"`, "Delete") : "")) +
        (open ? `<div class="flex flex-col gap-2 ps-4">${list ? entries("", list) : hint("Nobody yet.")}${add}</div>` : "");
    });
    const make = me.admin
      ? `<form class="grid gap-2" data-rform="group">${head("New group")}<input class="input" name="name" maxlength="200" required placeholder="Name" aria-label="Name of the group"><input class="input" name="description" maxlength="1000" placeholder="What it is for (optional)" aria-label="Description"><div>${submit("Make the group")}</div></form>`
      : "";
    return `${head("Groups")}${hint(me.admin ? "Every group of this workspace. Groups from the sign-in come with it and can only be read here." : "The groups you belong to.")}${rows.length ? entries("", rows.join("")) : hint("No group yet.")}${make}`;
  }

  function usersHtml() {
    if (!me.admin) {
      const p = st.people;
      if (p === null) return hint("Loading…");
      return `${head("Users")}${hint("Who works in this workspace. Administrators manage their rights.")}${entries("", p.people.map((x) => entry("", esc(x.name), esc(x.login), "")).join(""))}`;
    }
    load("users", "/api/users");
    if (st.users === null) return hint("Loading…");
    return `${head("Users")}${hint("Administrators manage rights, not content. Accounts of the gate or of single sign-on are administrators through their group at the sign-in.")}${entries("", st.users
      .map((u) => {
        const local = u.source === "local";
        const act = local ? small(`data-ract="admin" data-id="${esc(u.id)}" data-on="${u.admin ? "0" : "1"}"`, u.admin ? "Take back administrator" : "Make administrator") : "";
        return entry(`data-user="${esc(u.login)}"`, esc(u.name), `${esc(u.login)} · ${esc(u.source)}${u.admin ? " · ✓ administrator" : ""}${u.groups.length ? " · " + u.groups.map(esc).join(", ") : ""}`, act + small(`data-ract="heldby" data-id="${esc(u.id)}"`, "What may they do")) + (st.heldFor === u.id ? heldHtml() : "");
      })
      .join(""))}`;
  }

  function linksHtml() {
    load("links", "/api/links");
    if (st.links === null) return hint("Loading…");
    return `${head("Share links")}${hint(`${me.admin ? "Every link of this workspace." : "The links you made, and those on what you manage."} A link is made in the rights of a document, folder or file.`)}${linkRows(st.links, true)}`;
  }

  const MODEWORD = { read: "read", comment: "comment", edit: "edit", upload: "upload only" };
  function linkRows(links, withTitle, inDialog) {
    if (!links.length) return inDialog ? '<p class="text-sm text-muted-foreground">No link.</p>' : hint("No link.");
    return `<table class="table" data-links><thead><tr>${withTitle ? "<th>Of</th>" : ""}<th>Mode</th><th>Made by</th><th>Ends</th><th></th></tr></thead><tbody>${links
      .map(
        (l) =>
          `<tr data-link="${esc(l.id)}">${withTitle ? `<td>${esc(l.title)}</td>` : ""}<td>${MODEWORD[l.mode] || esc(l.mode)}${l.password ? " · 🔒 password" : ""}${l.users ? ` · ${l.users} opened` : ""}</td><td>${esc(l.creator)}<br><small>${when(l.created)}</small></td><td>${day(l.expires)}</td><td>${small(`data-ract="revoke" data-id="${esc(l.id)}"`, "Revoke", inDialog)}</td></tr>`,
      )
      .join("")}</tbody></table>`;
  }

  const ACTION = { open: "opened", change: "changed", grant: "role", inherit: "inheritance", owner: "owner", group: "group", member: "member", admin: "administrator",
    "link made": "link made", "link revoked": "link revoked", "link renewed": "link renewed", "link opened": "link opened", "link used": "link used",
    "link wrong password": "✗ wrong password of a link", "link upload": "upload through a link", "log kept": "log kept" };
  // named turns the IDs of accounts and groups in a line of the log into their names, where they are known.
  function named(text) {
    const p = st.people || { people: [], groups: [] };
    let out = esc(text);
    p.people.forEach((x) => (out = out.split(esc(x.id)).join(esc(x.name))));
    p.groups.forEach((g) => (out = out.split(esc(g.id)).join(esc(g.name))));
    return out;
  }
  // keepHtml says how long the log keeps an entry; administrators change it (ADR-29, 2026-11-19).
  function keepHtml() {
    load("keep", "/api/log/keep");
    if (st.keep === null) return "";
    if (!me.admin) return hint(`Entries are kept ${st.keep.days} days.`, " data-keep");
    // A number with its unit is C22's input group.
    return `<form class="flex flex-wrap items-center gap-2" data-rform="keep" data-keep><label class="text-sm" for="udkeep">Keep entries for</label><div class="input-group w-28"><input class="input" id="udkeep" type="number" name="days" min="${st.keep.min}" max="${st.keep.max}" value="${st.keep.days}" required aria-label="Days the log keeps an entry"><span data-align="inline-end">days</span></div>${submit("Save")}</form>`;
  }

  function logHtml() {
    load("log", "/api/log?limit=200");
    if (st.log === null) return hint("Loading…");
    return `${head("Log")}${hint(`${me.admin ? "Everything of this workspace." : "What you did, and what happened to what you manage."} Opening and changing count once an hour.`)}${keepHtml()}${
      st.log.length
        ? `${entries("data-log", st.log.map((e) => entry(`data-entry="${esc(e.action)}"`, `${ACTION[e.action] || esc(e.action)}${e.objectTitle ? ": " + esc(e.objectTitle) : ""}`, `${when(e.at)} · ${e.actor ? esc(e.actorName) : "someone with a link"}${e.detail ? " · " + named(e.detail) : ""}`, "")).join(""))}<div>${small('data-ract="reloadlog"', "Newer entries")}</div>`
        : hint("Nothing logged yet.")
    }`;
  }

  // heldHtml lists where the account asked for holds a role of its own; administrators transfer in place what it owns,
  // such as what someone left behind (ADR-29, 2026-11-19).
  function heldHtml() {
    return st.held
      ? st.held.length
        ? `${entries("data-held", st.held.map((x) => entry(`data-held-row="${esc(x.object)}"`, esc(x.title), `${ROLEWORD[x.role] || esc(x.role)} · ${VIA[x.via] || esc(x.via)}${x.name ? " " + esc(x.name) : ""}`, me.admin && x.via === "owner" && st.heldFor !== me.id ? `<form class="flex items-center gap-2" data-rform="transfer" data-id="${esc(x.object)}">${personSelect("to", "", true, st.heldFor)}${submit("Transfer")}</form>` : "")).join(""))}${me.admin && st.held.some((x) => x.via === "owner") && st.heldFor !== me.id ? hint("Whoever left owns what is listed with Transfer. Give it to someone else: you see its title and its rights, not its content.") : ""}`
        : hint("Nothing of their own: what they see comes from everyone.")
      : "";
  }

  function checkHtml() {
    const forWhom = me.admin ? `<form class="flex items-center gap-2" data-rform="held">${personSelect("account", st.heldFor || me.id)}${submit("Show")}</form>` : "";
    const held = heldHtml();
    const who = st.who
      ? `<div class="flex flex-col gap-2" data-who>${st.who.private ? hint(`Private: only ${esc(st.who.ownerName)} sees it.`) : `${hint(`Owner: ${esc(st.who.ownerName)}${st.who.inherit ? "" : " · takes no roles from above"}`)}${grantList(st.who.own, false)}${st.who.inherited.length ? `<h4 class="text-sm font-medium">From above</h4>${grantList(st.who.inherited, true)}` : ""}`}</div>`
      : "";
    const check = st.check ? `<p class="text-muted-foreground text-xs" data-check>${st.check.role ? `Role: ${ROLEWORD[st.check.role]}` : "No role: sees nothing of it"}${st.check.reasons.map((r) => ` · ${ROLEWORD[r.role]} ${VIA[r.via] || esc(r.via)}${r.name ? " " + esc(r.name) : ""} on ${esc(r.title)}`).join("")}</p>` : "";
    return `${head("What may someone do")}${forWhom}${held}
      ${head("Who sees something")}<form class="grid gap-2" data-rform="who"><input class="input" name="object" required placeholder="Address or ID, such as /o/7k2m9q4x1c" aria-label="Address of the object">${me.admin ? personSelect("account", me.id) : ""}<div>${submit("Check")}</div></form>${who}${check}`;
  }

  const grantName = (g) => (g.kind === "tenant" ? "Everyone" : g.kind === "group" ? `Group ${esc(g.name)}` : esc(g.name));
  const grantList = (list, from) =>
    list.length ? entries("", list.map((g) => entry("", grantName(g), ROLEWORD[g.role] || esc(g.role), "")).join("")) : hint("Nothing granted here.");

  // --- The rights dialog

  const dlg = { mode: "rights", id: "", title: "", rights: null, links: null, shareables: null, made: "" };
  // nw is the dialog that makes a collection at the top: its name, and with whom it is shared from the start. It is its
  // maker's alone unless they add someone here (ADR-29, 2026-11-19).
  const nw = { title: "", url: "", onMade: null, shares: [], busy: false, done: null };

  function dialogEl() {
    let d = document.getElementById("udrights");
    if (!d) {
      d = document.createElement("dialog");
      d.id = "udrights";
      d.className = "dialog";
      d.setAttribute("aria-labelledby", "udrights-title");
      document.body.appendChild(d);
    }
    return d;
  }

  async function open(objectId, title) {
    // The workspace keeps names escaped once; the dialog escapes what it shows itself.
    const plain = document.createElement("textarea");
    plain.innerHTML = title;
    dlg.mode = "rights";
    dlg.id = objectId;
    dlg.title = plain.value;
    dlg.rights = dlg.links = dlg.shareables = null;
    dlg.made = "";
    st.error = "";
    const d = dialogEl();
    drawDialog();
    if (!d.open) d.showModal();
    await refreshDialog();
  }

  async function refreshDialog() {
    if (dlg.mode === "new") {
      drawDialog();
      return;
    }
    try {
      if (st.people === null) st.people = await api("GET", "/api/people");
      dlg.rights = await api("GET", `/api/objects/${encodeURIComponent(dlg.id)}/rights`);
      if (dlg.rights.manage && !dlg.rights.private) {
        dlg.links = await api("GET", `/api/links?object=${encodeURIComponent(dlg.id)}`);
        dlg.shareables = await api("GET", `/api/objects/${encodeURIComponent(dlg.id)}/shareables`);
      }
    } catch (e) {
      st.error = e.message;
    }
    drawDialog();
  }

  function drawDialog() {
    if (dlg.mode === "new") {
      drawNew();
      return;
    }
    const d = dialogEl();
    const r = dlg.rights;
    let body = '<p class="text-sm text-muted-foreground">Loading…</p>';
    if (r && r.private) body = `<p class="text-sm text-muted-foreground">Private: only ${esc(r.ownerName)} sees it. Nothing private is shared, also not with administrators.</p>`;
    else if (r) {
      const may = r.manage;
      const row = "flex flex-wrap items-center justify-between gap-2 mt-2";
      const owner = `<div class="${row}"><span>Owner: <b data-owner>${esc(r.ownerName)}</b></span>${r.role === "owner" || me.admin ? `<form class="flex items-center gap-2" data-rform="owner">${personSelect("to", r.owner)}<button class="btn" data-variant="outline" data-size="sm" type="submit">Transfer</button></form>` : ""}</div>`;
      // A file in a library shared over Samba keeps the rights of its folder on the share (ADR-29, ADR-30): it cannot
      // break the inheritance, and what is granted on it holds in the web only.
      const sm = r.samba || null;
      const perFolder = sm && sm.shared && sm.file;
      const mayBreak = may && !(perFolder && r.inherit);
      const inherit = `<div class="${row}"><span>${r.inherit ? "Takes the roles from above" : "Takes no roles from above"}</span>${mayBreak ? small(`data-ract="inherit" data-on="${r.inherit ? "0" : "1"}"`, r.inherit ? "Break the inheritance" : "Take from above again", true) : ""}</div>${r.inherit ? "" : '<p class="text-sm text-muted-foreground">Breaking copied what held from above; take back here what shall not hold.</p>'}`;
      const samba = sm
        ? `${perFolder ? '<p class="text-sm text-muted-foreground" data-samba-per-folder>Samba knows rights per folder only: roles granted on this file hold in the web only, and the share shows it as its folder does.</p>' : ""}${sm.path ? `<p class="text-sm" data-samba-path>On the share: <code>${esc(sm.path)}</code></p>` : ""}${sm.root !== undefined && sm.root !== null ? `<div class="${row}" data-samba-root><span>${sm.root ? "Shared over Samba" : "Not shared over Samba"}</span>${may ? small(`data-ract="samba" data-on="${sm.root ? "0" : "1"}"`, sm.root ? "Stop sharing over Samba" : "Share over Samba", true) : ""}</div>` : ""}`
        : "";
      const own = r.own.length
        ? `<table class="table" data-grants><tbody>${r.own
            .map(
              (g) =>
                `<tr data-grant="${esc(g.kind)}:${esc(g.who)}"><td>${grantName(g)}</td><td>${may ? `<form data-rform="role" data-kind="${esc(g.kind)}" data-who="${esc(g.who)}">${roleSelect("role", g.role)}</form>` : ROLEWORD[g.role]}</td><td>${may ? small(`data-ract="ungrant" data-kind="${esc(g.kind)}" data-who="${esc(g.who)}"`, "Take back", true) : ""}</td></tr>`,
            )
            .join("")}</tbody></table>`
        : '<p class="text-sm text-muted-foreground">Nothing granted here.</p>';
      const add = may ? `<form class="flex items-center gap-2 mt-2" data-rform="grant">${whoSelect("who", true)}${roleSelect("role", "read")}<button class="btn" data-size="sm" type="submit">Grant</button></form>` : "";
      const inherited = r.inherited.length
        ? `<h3 class="font-medium mt-3">From above</h3><table class="table" data-inherited><tbody>${r.inherited.map((g) => `<tr><td>${grantName(g)}</td><td>${ROLEWORD[g.role] || esc(g.role)}</td></tr>`).join("")}</tbody></table>`
        : "";
      body = `${owner}${inherit}${samba}<h3 class="font-medium mt-3">Roles here</h3>${own}${add}${inherited}${may ? linksPart() : ""}`;
    }
    d.innerHTML = `<div><header><h2 id="udrights-title">Rights: ${esc(dlg.title)}</h2><p>Roles for people, groups and everyone, inherited by what lies below. Only allowing: narrower means breaking the inheritance.</p></header><section class="max-h-[420px] overflow-y-auto">${problem()}${body}</section><footer><button class="btn" data-variant="outline" data-ract="close">Close</button></footer></div>`;
  }

  // --- A new collection, and with whom it is shared from the start

  // create opens the dialog that makes a collection at the top through url (POST {title}). With whom it is shared is
  // granted afterwards through the same API as the rights dialog; whatever is not granted stays its maker's alone.
  // onMade gets what the server answered for the collection.
  async function create(opts) {
    dlg.mode = "new";
    nw.title = opts.title || "";
    nw.url = opts.url;
    nw.onMade = opts.onMade || null;
    nw.shares = [];
    nw.busy = false;
    nw.done = null;
    st.error = "";
    const d = dialogEl();
    drawNew();
    if (!d.open) d.showModal();
    const field = d.querySelector("[data-new-title]");
    if (field) {
      field.focus();
      field.select();
    }
    if (st.people === null) {
      try {
        st.people = await api("GET", "/api/people");
      } catch (e) {
        st.error = e.message;
      }
      drawNew();
    }
  }

  // keepFocus draws the dialog again while the name keeps its focus, its selection and what was typed: the people come
  // later than the dialog opens, and redrawing it took the focus away while someone typed (CI of #92, 2026-11-19).
  function keepFocus(draw) {
    const was = document.activeElement;
    const focused = was && was.matches && was.matches("[data-new-title]") ? [was.selectionStart, was.selectionEnd] : null;
    draw();
    if (!focused) return;
    const field = document.querySelector("#udrights [data-new-title]");
    if (field) {
      field.focus();
      field.setSelectionRange(focused[0], focused[1]);
    }
  }

  const shareName = (x) => {
    const p = st.people || { people: [], groups: [] };
    if (x.kind === "tenant") return "Everyone";
    if (x.kind === "group") return `Group ${esc((p.groups.find((g) => g.id === x.who) || { name: x.who }).name)}`;
    return esc((p.people.find((q) => q.id === x.who) || { name: x.who }).name);
  };

  function drawNew() {
    keepFocus(paintNew);
  }

  function paintNew() {
    const d = dialogEl();
    const list = nw.shares.length
      ? `<table class="table" data-new-shares><tbody>${nw.shares
          .map(
            (x, i) =>
              `<tr data-new-share="${esc(x.kind)}:${esc(x.who)}"><td>${shareName(x)}</td><td>${roleSelect("nrole" + i, x.role, nw.busy).replace("data-rsel=", `data-nshare="${i}" data-rsel=`)}</td><td>${small(`data-ract="newunshare" data-i="${i}"${nw.busy ? " disabled" : ""}`, "Take out", true)}</td></tr>`,
          )
          .join("")}</tbody></table>`
      : '<p class="text-sm text-muted-foreground" data-only-me>Only you. Nobody else sees it until you share it, here or later in its rights.</p>';
    const body = nw.done
      ? `<p class="text-sm" data-new-done>${esc(nw.done)}</p>`
      : `<form class="grid gap-2" id="udnew" data-rform="new"><label class="label" for="udnew-title">Name</label><input class="input" id="udnew-title" name="title" data-new-title required maxlength="200" value="${esc(nw.title)}"${nw.busy ? " disabled" : ""}>
        <fieldset class="grid gap-2 mt-2" data-share-step><legend class="font-medium">Share with</legend>${list}
        <div class="flex flex-wrap items-center gap-2">${whoSelect("who", true)}${roleSelect("role", "read")}${small('data-ract="newshare"' + (nw.busy ? " disabled" : ""), "Add", true)}</div></fieldset></form>`;
    const foot = nw.done
      ? '<button class="btn" data-variant="outline" data-ract="close" type="button">Close</button>'
      : `<button class="btn" data-variant="outline" data-ract="close" type="button">Cancel</button><button class="btn" type="submit" form="udnew"${nw.busy ? " disabled" : ""}>Make</button>`;
    d.innerHTML = `<div><header><h2 id="udrights-title">New collection</h2><p>It is yours alone, not private: share it with everyone, groups or people now or later. Each holds the role given and passes it to what lies inside.</p></header><section class="max-h-[420px] overflow-y-auto">${problem()}${body}</section><footer>${foot}</footer></div>`;
  }

  // make sends the new collection, then grants each share; a share that fails leaves the collection less shared, never
  // more, and says so.
  async function make() {
    nw.busy = true;
    st.error = "";
    drawNew();
    let made;
    try {
      made = await api("POST", nw.url, { title: nw.title.trim() });
    } catch (e) {
      st.error = e.message;
      nw.busy = false;
      drawNew();
      return;
    }
    const failed = [];
    for (const x of nw.shares) {
      try {
        await api("PUT", `/api/objects/${encodeURIComponent(made.id)}/grants`, { kind: x.kind, who: x.who, role: x.role });
      } catch (e) {
        failed.push(`${shareName(x)}: ${e.message}`);
      }
    }
    nw.busy = false;
    st.log = null;
    if (nw.onMade) nw.onMade(made);
    if (failed.length) {
      nw.done = `Made, but not shared with ${failed.join(", ")}. Its rights share it.`;
      drawNew();
      return;
    }
    dialogEl().close();
    redraw();
  }

  function linksPart() {
    if (dlg.links === null) return "";
    const made = dlg.made
      ? `<div class="input-group mt-2" data-made-link><input class="input" readonly value="${esc(dlg.made)}" aria-label="The address of the new link"><button class="btn" data-variant="outline" data-ract="copy" type="button">Copy</button></div><p class="text-sm text-muted-foreground">Shown this once: only its hash is kept.</p>`
      : "";
    const release = dlg.shareables && dlg.shareables.length
      ? `<fieldset class="grid gap-2"><legend class="text-sm">Embedded here, shared only if released</legend>${dlg.shareables.map((s) => `<label class="label"><input type="checkbox" name="release" value="${esc(s.id)}"> ${esc(s.title)}</label>`).join("")}</fieldset>`
      : "";
    return `<h3 class="font-medium mt-3">Share links</h3>${linkRows(dlg.links, false, true)}${made}
      <form class="grid gap-2 mt-2" data-rform="link"><div class="grid grid-cols-2 gap-2"><select class="select" name="mode" aria-label="Mode">${MODES.map((m) => `<option value="${m[0]}">${m[1]}</option>`).join("")}</select>
      <input class="input" type="date" name="expires" aria-label="Ends on"></div><input class="input" type="password" name="password" autocomplete="new-password" placeholder="Password (optional)" aria-label="Password of the link">${release}<button class="btn" type="submit">Make a link</button></form>`;
  }

  // --- What a person does

  async function act(fn, ...keys) {
    st.error = "";
    try {
      await fn();
    } catch (e) {
      st.error = e.message;
    }
    st.log = null;
    if (keys.length) reload(...keys);
    else redraw();
    if (dialogEl().open) await refreshDialog();
  }

  const objectPath = (suffix) => `/api/objects/${encodeURIComponent(dlg.id)}${suffix}`;
  const idOf = (address) => {
    const m = String(address).trim().match(/([0-9a-hjkmnp-tv-z]{10})/);
    return m ? m[1] : "";
  };

  document.addEventListener("click", (e) => {
    const b = e.target.closest("[data-ract]");
    if (!b) return;
    const a = b.dataset;
    switch (a.ract) {
      case "close":
        dialogEl().close();
        redraw();
        return;
      case "copy": {
        const input = b.parentElement.querySelector("input");
        (navigator.clipboard ? navigator.clipboard.writeText(input.value) : Promise.reject()).catch(() => input.select());
        return;
      }
      case "members":
        if (st.members[a.g]) {
          delete st.members[a.g];
          redraw();
          return;
        }
        api("GET", `/api/groups/${encodeURIComponent(a.g)}/members`).then(
          (m) => {
            st.members[a.g] = m;
            redraw();
          },
          (err) => {
            st.error = err.message;
            redraw();
          },
        );
        return;
      case "unmember":
        act(() => api("PUT", `/api/groups/${encodeURIComponent(a.g)}/members/${encodeURIComponent(a.id)}`, { member: false }), "groups").then(() => b.dataset.g && refreshMembers(a.g));
        return;
      case "delgroup":
        if (!window.confirm(`Delete the group ${a.name}? Every role granted to it goes as well.`)) return;
        act(() => api("DELETE", `/api/groups/${encodeURIComponent(a.g)}`), "groups", "people");
        return;
      case "admin":
        act(() => api("PUT", `/api/users/${encodeURIComponent(a.id)}/admin`, { admin: a.on === "1" }), "users");
        return;
      case "heldby":
        st.heldFor = a.id;
        act(async () => {
          st.held = await api("GET", `/api/rights/held?account=${encodeURIComponent(a.id)}`);
        });
        return;
      case "reloadlog":
        reload("log");
        return;
      case "newshare": {
        const f = b.closest("[data-share-step]");
        const [kind, who] = f.querySelector("select[name=who]").value.split(/:(.*)/s);
        const role = f.querySelector("select[name=role]").value;
        const at = nw.shares.findIndex((x) => x.kind === kind && x.who === who);
        if (at >= 0) nw.shares[at].role = role;
        else nw.shares.push({ kind, who, role });
        drawNew();
        return;
      }
      case "newunshare":
        nw.shares.splice(Number(a.i), 1);
        drawNew();
        return;

      case "revoke":
        if (!window.confirm("Revoke this link? It leads nowhere from then on.")) return;
        act(() => api("DELETE", `/api/links/${encodeURIComponent(a.id)}`), "links");
        return;
      case "inherit":
        act(() => api("PUT", objectPath("/inherit"), { inherit: a.on === "1" }));
        return;
      case "samba":
        act(() => api("PUT", `/api/files/roots/${encodeURIComponent(dlg.id)}/samba`, { on: a.on === "1" }));
        return;
      case "ungrant":
        act(() => api("PUT", objectPath("/grants"), { kind: a.kind, who: a.who, role: "" }));
        return;
    }
  });

  async function refreshMembers(groupID) {
    try {
      st.members[groupID] = await api("GET", `/api/groups/${encodeURIComponent(groupID)}/members`);
    } catch (e) {
      st.error = e.message;
    }
    redraw();
  }

  // Enter in the name makes the new collection, Escape closes the dialog without making anything.
  document.addEventListener("keydown", (e) => {
    const d = document.getElementById("udrights");
    if (!d || !d.open || dlg.mode !== "new" || nw.done) return;
    if (e.key === "Enter" && e.target.matches && e.target.matches("[data-new-title]")) {
      e.preventDefault();
      const f = document.getElementById("udnew");
      if (f) f.requestSubmit();
    } else if (e.key === "Escape") {
      e.preventDefault();
      d.close();
      redraw();
    }
  });

  document.addEventListener("input", (e) => {
    if (e.target.matches && e.target.matches("[data-new-title]")) nw.title = e.target.value;
  });

  document.addEventListener("change", (e) => {
    const share = e.target.closest("[data-nshare]");
    if (share) {
      nw.shares[Number(share.dataset.nshare)].role = share.value;
      return;
    }
    const sel = e.target.closest("[data-rform=role] select");
    if (!sel) return;
    const f = sel.form;
    act(() => api("PUT", objectPath("/grants"), { kind: f.dataset.kind, who: f.dataset.who, role: sel.value }));
  });

  document.addEventListener("submit", (e) => {
    const f = e.target.closest("form[data-rform]");
    if (!f) return;
    e.preventDefault();
    const v = (n) => (f.elements[n] ? f.elements[n].value : "");
    switch (f.dataset.rform) {
      case "keep":
        act(() => api("PUT", "/api/log/keep", { days: Number(v("days")) }), "keep");
        return;
      case "new":
        nw.title = v("title");
        if (!nw.busy && nw.title.trim()) make();
        return;
      case "transfer":
        if (!v("to")) return;
        if (!window.confirm("Give this to someone else? Whoever had it keeps the role edit. You see nothing of it unless you hold a role.")) return;
        act(async () => {
          await api("PUT", `/api/objects/${encodeURIComponent(f.dataset.id)}/owner`, { to: v("to") });
          st.held = await api("GET", `/api/rights/held?account=${encodeURIComponent(st.heldFor)}`);
        });
        return;
      case "group":
        act(() => api("POST", "/api/groups", { name: v("name"), description: v("description") }), "groups", "people");
        return;
      case "member":
        act(() => api("PUT", `/api/groups/${encodeURIComponent(f.dataset.g)}/members/${encodeURIComponent(v("account"))}`, { member: true }), "groups").then(() => refreshMembers(f.dataset.g));
        return;
      case "held":
        st.heldFor = v("account");
        act(async () => {
          st.held = await api("GET", `/api/rights/held?account=${encodeURIComponent(v("account"))}`);
        });
        return;
      case "who": {
        const objectId = idOf(v("object"));
        act(async () => {
          st.who = await api("GET", `/api/objects/${encodeURIComponent(objectId)}/rights`);
          st.check = await api("GET", `/api/rights/check?object=${encodeURIComponent(objectId)}&account=${encodeURIComponent(v("account") || me.id)}`);
        });
        return;
      }
      case "owner":
        if (!window.confirm("Give this to someone else? You keep the role edit.")) return;
        act(() => api("PUT", objectPath("/owner"), { to: v("to") }));
        return;
      case "grant": {
        const [kind, who] = v("who").split(/:(.*)/s);
        act(() => api("PUT", objectPath("/grants"), { kind, who, role: v("role") }));
        return;
      }
      case "link": {
        const released = [...f.querySelectorAll("input[name=release]:checked")].map((x) => x.value);
        act(async () => {
          const made = await api("POST", objectPath("/links"), { mode: v("mode"), expires: v("expires"), password: v("password"), released });
          dlg.made = made.url;
        });
        return;
      }
    }
  });

  return {
    SECTIONS,
    nav3,
    open,
    create,
    setup(render, user) {
      redraw = render;
      me = user || me;
    },
  };
})();
