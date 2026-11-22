// SPDX-License-Identifier: MIT

// Passwords in the text (ADR-7, docs/secrets.md). A page shows each secret as a mask with Reveal and Copy (the server
// draws it, internal/web/api_secrets.go); the value comes from the server on a click only and never stays: a revealed
// value is masked again after half a minute or when the page is drawn again, and Copy puts it on the clipboard without
// showing it. The editor makes, reveals, copies and changes secrets through window.UDSecrets, and the settings of a page
// list its secrets with versions, readers and deletion in a dialog of their own.
(() => {
  'use strict';

  const SHOWN_MS = 30000;
  const MASK = '••••••••';

  async function api(method, url, body) {
    const init = { method, credentials: 'same-origin', headers: { Accept: 'application/json' } };
    if (body !== undefined) {
      init.headers['Content-Type'] = 'application/json';
      init.body = JSON.stringify(body);
    }
    const r = await fetch(url, init);
    let data = null;
    if (r.status !== 204) {
      try {
        data = await r.json();
      } catch (e) {
        data = null;
      }
    }
    if (!r.ok) {
      const err = new Error((data && data.error) || String(r.status));
      err.status = r.status;
      throw err;
    }
    return data;
  }

  const esc = (s) => String(s).replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c]);
  const sid = (id) => encodeURIComponent(id);

  function say(text) {
    if (window.UDToast) window.UDToast(text);
    else window.alert(text);
  }

  function why(err) {
    if (err.status === 404) return 'This password is not available to you.';
    if (err.status === 429 || err.status === 503) return err.message;
    if (err.status === 403) return err.message || 'Others may not do this.';
    return 'The password could not be fetched.';
  }

  /** reveal fetches the value of a secret: purpose "show" or "copy", version 0 for the newest. */
  const reveal = (id, purpose, version) => api('POST', `/api/secrets/${sid(id)}/reveal`, { purpose, version: version || 0 }).then((d) => d.value);

  /** copy puts the value on the clipboard without showing it. */
  async function copy(id, version) {
    const value = await reveal(id, 'copy', version);
    if (!navigator.clipboard) throw Object.assign(new Error('The browser does not let this page copy.'), { status: 403 });
    await navigator.clipboard.writeText(value);
  }

  /** showIn puts a value into a mask for a while, and masks it again. */
  function showIn(mask, button, value) {
    mask.textContent = value;
    mask.dataset.shown = '1';
    if (button) button.textContent = 'Hide';
    clearTimeout(mask._hide);
    mask._hide = setTimeout(() => hideIn(mask, button), SHOWN_MS);
  }

  function hideIn(mask, button) {
    clearTimeout(mask._hide);
    mask.textContent = MASK;
    delete mask.dataset.shown;
    if (button) button.textContent = 'Reveal';
  }

  function flash(button, text) {
    const was = button.textContent;
    button.textContent = text;
    setTimeout(() => {
      if (button.textContent === text) button.textContent = was;
    }, 2000);
  }

  /** toggle reveals a secret into its mask, or masks it again. */
  async function toggle(id, mask, button) {
    if (mask.dataset.shown) {
      hideIn(mask, button);
      return;
    }
    try {
      showIn(mask, button, await reveal(id, 'show'));
    } catch (err) {
      say(why(err));
    }
  }

  async function copyFrom(id, button) {
    try {
      await copy(id);
      if (button) flash(button, 'Copied');
    } catch (err) {
      say(why(err));
    }
  }

  // --- Asking for a value, in a dialog of its own (the editor and the settings of a page)

  function askEl() {
    let d = document.getElementById('udsecret-ask');
    if (!d) {
      d = document.createElement('dialog');
      d.id = 'udsecret-ask';
      d.className = 'dialog';
      d.setAttribute('aria-labelledby', 'udsecret-ask-title');
      document.body.appendChild(d);
    }
    return d;
  }

  /** ask asks for a value without showing it, and resolves to it, or to null when cancelled. */
  function ask(title) {
    const d = askEl();
    d.innerHTML = `<form data-secret-ask><header><h2 id="udsecret-ask-title">${esc(title)}</h2><p>Kept sealed on the server. The page holds only a reference; the value comes on a click, and every time is logged.</p></header><section><div class="field"><label class="label" for="udsecret-value">Password</label><input class="input" id="udsecret-value" type="password" autocomplete="off" maxlength="16384" required data-secret-input></div></section><footer><button class="btn" data-variant="outline" type="button" data-secret-cancel>Cancel</button><button class="btn" type="submit">Keep</button></footer></form>`;
    return new Promise((resolve) => {
      const form = d.querySelector('form');
      const done = (value) => {
        form.reset();
        if (d.open) d.close();
        resolve(value);
      };
      form.addEventListener('submit', (e) => {
        e.preventDefault();
        const value = d.querySelector('[data-secret-input]').value;
        done(value ? value : null);
      });
      d.querySelector('[data-secret-cancel]').addEventListener('click', () => done(null));
      d.addEventListener('cancel', () => done(null), { once: true });
      if (!d.open) d.showModal();
      d.querySelector('[data-secret-input]').focus();
    });
  }

  /** make asks for a value, makes a secret of the page and resolves to its reference for the text, or to null. */
  async function make(page) {
    const value = await ask('New password');
    if (value === null) return null;
    try {
      return (await api('POST', `/api/pages/${sid(page)}/secrets`, { value })).ref;
    } catch (err) {
      say(err.status === 403 ? 'Only who may change this page adds passwords.' : err.message);
      return null;
    }
  }

  /**
   * makeFrom makes a secret of the page from a value typed as text, which the editor found before saving (ADR-7,
   * 2026-11-20), and resolves to its reference, or to null.
   */
  async function makeFrom(page, value) {
    try {
      return (await api('POST', `/api/pages/${sid(page)}/secrets`, { value })).ref;
    } catch (err) {
      say(err.status === 403 ? 'Only who may change this page adds passwords.' : err.message);
      return null;
    }
  }

  /** change asks for a new value of a secret and keeps it as a new version. */
  async function change(id) {
    const value = await ask('Change the password');
    if (value === null) return false;
    try {
      await api('PUT', `/api/secrets/${sid(id)}`, { value });
      say('The password was changed; the old one is a version.');
      return true;
    } catch (err) {
      say(why(err));
      return false;
    }
  }

  // --- Reading: the masks the server drew

  // In the capture phase, before the workspace's own handler of clicks (demo.js, also capturing, loaded later), which
  // would draw the page again under the click; a click on a mask is not one of its.
  document.addEventListener(
    'click',
    (e) => {
      const b = e.target.closest && e.target.closest('[data-pwact]');
      if (!b) return;
      e.preventDefault();
      e.stopImmediatePropagation();
      pwClick(b);
    },
    true,
  );

  function pwClick(b) {
    const id = b.dataset.secret;
    const box = b.closest('[data-secret-state]');
    const mask = box && box.querySelector('[data-secret-value]');
    switch (b.dataset.pwact) {
      case 'show':
        if (mask) toggle(id, mask, b);
        return;
      case 'copy':
        copyFrom(id, b);
        return;
      default:
        dialogAct(b);
    }
  }

  // --- The secrets of a page, from its settings

  const dlg = { page: '', title: '', list: null, vault: true, open: {}, people: null, error: '' };

  function dialogEl() {
    let d = document.getElementById('udsecrets');
    if (!d) {
      d = document.createElement('dialog');
      d.id = 'udsecrets';
      d.className = 'dialog';
      d.setAttribute('aria-labelledby', 'udsecrets-title');
      document.body.appendChild(d);
    }
    return d;
  }

  async function open(page, title) {
    const plain = document.createElement('textarea');
    plain.innerHTML = title;
    Object.assign(dlg, { page, title: plain.value, list: null, open: {}, error: '' });
    const d = dialogEl();
    draw();
    if (!d.open) d.showModal();
    await refresh();
  }

  async function refresh() {
    try {
      const got = await api('GET', `/api/pages/${sid(dlg.page)}/secrets`);
      dlg.vault = got.vault;
      dlg.list = got.secrets;
      for (const id of Object.keys(dlg.open)) dlg.open[id] = await api('GET', `/api/secrets/${sid(id)}`);
      dlg.error = '';
    } catch (err) {
      dlg.error = why(err);
    }
    draw();
  }

  const btn = (attrs, label) => `<button class="btn" data-variant="outline" data-size="sm" type="button" ${attrs}>${esc(label)}</button>`;
  const when = (iso) => (iso ? new Date(iso).toLocaleString() : '');

  function readersForm(s) {
    const people = dlg.people || { people: [], groups: [] };
    const named = new Set((s.readers || []).map((r) => `${r.kind}:${r.who}`));
    const box = (kind, x) => `<label class="label"><input class="input" type="checkbox" data-secret-who value="${esc(kind)}:${esc(x.id)}"${named.has(`${kind}:${x.id}`) ? ' checked' : ''}> ${esc(kind === 'group' ? `Group ${x.name}` : x.name || x.login)}</label>`;
    return `<form class="grid gap-2" data-secret-readers="${esc(s.id)}"><label class="label"><input class="input" type="checkbox" data-secret-restricted${s.restricted ? ' checked' : ''}> Only these people and groups, as far as they read the page</label><div class="grid gap-1" role="group" aria-label="Who may reveal it">${people.people.map((p) => box('person', p)).join('')}${people.groups.map((g) => box('group', g)).join('')}</div><div>${btn('data-pwact="readers" data-secret="' + esc(s.id) + '"', 'Keep who may see it')}</div></form>`;
  }

  function detail(s) {
    const info = dlg.open[s.id];
    if (!info) return '';
    const versions = (info.versions || [])
      .map(
        (v) =>
          `<tr data-secret-version="${v.number}"><td>${v.number}</td><td>${esc(when(v.at))}</td><td>${esc(v.who)}${v.origin ? ` <small class="text-muted-foreground">${esc(v.origin)}</small>` : ''}</td><td>${btn(`data-pwact="vcopy" data-secret="${esc(s.id)}" data-n="${v.number}"`, 'Copy')}${v.number === info.versions[0].number ? '' : btn(`data-pwact="restore" data-secret="${esc(s.id)}" data-n="${v.number}"`, 'Restore')}</td></tr>`,
      )
      .join('');
    const readers = info.restricted ? `<p class="text-sm">Only: ${(info.readers || []).map((r) => esc(r.name || r.who)).join(', ') || 'nobody'}</p>` : '<p class="text-sm text-muted-foreground">Whoever reads the page may reveal it.</p>';
    return `<div class="grid gap-2 ps-4" data-secret-detail="${esc(s.id)}">${readers}${info.mayRestrict ? readersForm(info) : ''}${versions ? `<h3 class="text-sm font-medium">Versions</h3><div class="table-container"><table class="table"><tbody>${versions}</tbody></table></div>` : ''}</div>`;
  }

  function draw() {
    const d = dialogEl();
    let body = '<p class="text-sm text-muted-foreground">Loading…</p>';
    if (dlg.list) {
      body = dlg.list.length
        ? dlg.list
            .map((s) => {
              const state = s.may ? (s.restricted ? 'restricted' : '') : 'hidden';
              const head = `<section><h4 class="font-medium">Password <code>${esc(s.id)}</code>${s.referenced ? '' : ' <span class="badge" data-variant="muted">not in the text</span>'}${state ? ` <span class="badge" data-variant="outline">${state}</span>` : ''}</h4>${s.changed ? `<p class="text-muted-foreground text-sm">Changed ${esc(when(s.changed))}</p>` : ''}</section>`;
              const acts = s.may
                ? `${btn(`data-pwact="dcopy" data-secret="${esc(s.id)}"`, 'Copy')}${s.mayChange ? btn(`data-pwact="change" data-secret="${esc(s.id)}"`, 'Change') : ''}${s.mayChange || s.mayRestrict ? btn(`data-pwact="more" data-secret="${esc(s.id)}"`, dlg.open[s.id] ? 'Less' : 'Versions and readers') : ''}${s.mayChange ? btn(`data-pwact="delete" data-secret="${esc(s.id)}"`, 'Delete') : ''}`
                : s.mayRestrict
                  ? btn(`data-pwact="more" data-secret="${esc(s.id)}"`, dlg.open[s.id] ? 'Less' : 'Readers')
                  : '';
              return `<div class="grid gap-2" role="listitem" data-secret-row="${esc(s.id)}"><article class="item" data-variant="outline" data-size="sm">${head}${acts ? `<aside class="flex flex-wrap items-center gap-2">${acts}</aside>` : ''}</article>${detail(s)}</div>`;
            })
            .join('')
        : '<p class="text-sm text-muted-foreground">No passwords on this page. Add one with Password in the bar or the slash menu of the editor.</p>';
    }
    const vault = dlg.vault ? '' : '<p class="text-sm" data-secret-novault>Passwords need a key on the server; none is set, so none can be made or revealed.</p>';
    const problem = dlg.error ? `<p class="text-sm" role="alert">${esc(dlg.error)}</p>` : '';
    // From C22's parts, as the rights dialog: each password an .item with its actions, what opens below it indented; the
    // section scrolls within the height C22 gives a dialog.
    if (dlg.list && dlg.list.length) body = `<div class="item-group" role="list">${body}</div>`;
    d.innerHTML = `<div><header><h2 id="udsecrets-title">Passwords: ${esc(dlg.title)}</h2><p>Kept sealed on the server, revealed on a click and logged. Operations documentation, not a vault.</p></header><section class="scrollbar -mx-4 grid min-h-0 gap-4 overflow-y-auto px-4">${problem}${vault}${body}</section><footer><button class="btn" data-variant="outline" type="button" data-pwact="close">Close</button></footer></div>`;
  }

  async function dialogAct(b) {
    const id = b.dataset.secret;
    const n = +b.dataset.n || 0;
    try {
      switch (b.dataset.pwact) {
        case 'close':
          dialogEl().close();
          return;
        case 'dcopy':
        case 'vcopy':
          await copy(id, n);
          flash(b, 'Copied');
          return;
        case 'change':
          if (await change(id)) await refresh();
          return;
        case 'more':
          if (dlg.open[id]) delete dlg.open[id];
          else {
            dlg.open[id] = await api('GET', `/api/secrets/${sid(id)}`);
            if (dlg.open[id].mayRestrict && !dlg.people) dlg.people = await api('GET', '/api/people');
          }
          draw();
          return;
        case 'restore':
          if (!window.confirm(`Make version ${n} the password again? It becomes a new version.`)) return;
          await api('POST', `/api/secrets/${sid(id)}/versions/${n}/restore`);
          await refresh();
          return;
        case 'delete':
          if (!window.confirm('Delete this password with every version? A reference left in the text shows that it is not available.')) return;
          await api('DELETE', `/api/secrets/${sid(id)}`);
          delete dlg.open[id];
          await refresh();
          return;
        case 'readers': {
          const form = b.closest('form');
          const restricted = form.querySelector('[data-secret-restricted]').checked;
          const readers = restricted
            ? [...form.querySelectorAll('[data-secret-who]:checked')].map((o) => {
                const [kind, who] = o.value.split(/:(.*)/s);
                return { kind, who };
              })
            : [];
          await api('PUT', `/api/secrets/${sid(id)}/readers`, { restricted, readers });
          await refresh();
          return;
        }
      }
    } catch (err) {
      dlg.error = why(err);
      draw();
    }
  }

  window.UDSecrets = {
    open,
    make,
    makeFrom,
    change,
    reveal,
    copy,
    /** forEditor gives the editor of a page what its masks need. */
    forEditor: () => ({ show: (id) => reveal(id, 'show'), copy: (id) => copy(id), change: (id) => change(id), why }),
    MASK,
    SHOWN_MS,
  };
})();
