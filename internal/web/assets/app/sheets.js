/* SPDX-License-Identifier: MIT */
/* Spreadsheets as tables (ADR-31): the table view of a file. The workspace (demo.js) draws its window as HTML text again
   and again; this script keeps one view per file as DOM and puts it back into the placeholder <div data-sheets="<id>">
   each time, as the PDF preview does. Every text from the file goes in through textContent, never as HTML.

   The view: the visible sheets as a segmented control, a head of column letters and a column of row numbers that stay while the table
   scrolls, rows in pages of 500, a range marked with the mouse (drag, Shift+click) or the keyboard (arrows, Shift+arrows),
   and for the marked range, a sheet, a name or an Excel table: Embed (its line goes to the clipboard) and Download CSV.
   Below, the tables of the file that pages use, with what became of each in the current version. */
(function () {
  'use strict';
  const VIEWS = {};
  const pending = {};
  const json = async (url, opts) => {
    const r = await fetch(url, Object.assign({ credentials: 'same-origin', headers: { Accept: 'application/json' } }, opts || {}));
    const a = await r.json().catch(() => ({}));
    if (!r.ok) throw new Error(a.error || String(r.status));
    return a;
  };
  const el = (tag, cls, text) => {
    const n = document.createElement(tag);
    if (cls) n.className = cls;
    if (text !== undefined) n.textContent = text;
    return n;
  };
  const colName = c => { let s = ''; while (c > 0) { c--; s = String.fromCharCode(65 + c % 26) + s; c = Math.floor(c / 26); } return s; };
  const cellName = (r, c) => colName(c) + r;
  const say = text => { if (window.udToast) window.udToast(text); else { const n = document.querySelector('[data-sheet-say]'); if (n) n.textContent = text; } };
  const api = id => '/api/files/' + encodeURIComponent(id);

  /* A view of one file: its sheets, the shown sheet and page, the marked range, and its DOM. */
  function view(id) {
    if (VIEWS[id]) return VIEWS[id];
    const v = VIEWS[id] = { id, meta: null, sheet: '', from: 0, grid: null, sel: null, anchor: null, tables: [], root: el('div', ''), failed: '' };
    v.root.dataset.sheetview = id;
    v.root.append(el('p', 'lab', 'Loading the tables…'));
    load(v);
    return v;
  }

  async function load(v) {
    try {
      v.meta = await json(api(v.id) + '/sheets');
      const want = pending[v.id];
      delete pending[v.id];
      v.sheet = want && v.meta.sheets.some(s => s.name === want.sheet) ? want.sheet : (v.meta.sheets[0] || {}).name || '';
      await page(v, want && want.sheet === v.sheet ? { at: want.row } : { from: 0 });
      v.mark = want && want.sheet === v.sheet ? want.row : 0;
      await tables(v);
    } catch (e) { v.failed = e.message; }
    draw(v);
  }

  async function page(v, { at, from }) {
    if (!v.sheet) { v.grid = null; return; }
    const q = new URLSearchParams({ sheet: v.sheet });
    if (at) q.set('at', at); else q.set('from', from || 0);
    v.grid = await json(api(v.id) + '/sheets/grid?' + q);
    v.from = v.grid.from;
    v.sel = null;
  }

  async function tables(v) {
    try { v.tables = await json(api(v.id) + '/tables'); } catch (e) { v.tables = []; }
  }

  const STATE = { ok: 'shows as chosen', moved: 'followed to its new place', changed: 'its first row changed: check it', lost: 'no longer to be found' };

  function draw(v) {
    const root = v.root;
    root.replaceChildren();
    if (v.failed) { root.append(el('div', 'note', 'This file shows no tables: ' + v.failed)); return; }
    if (!v.meta) { root.append(el('p', 'lab', 'Loading the tables…')); return; }
    if (!v.meta.sheets.length) { root.append(el('div', 'note', 'This workbook has no visible sheet.')); return; }
    /* The sheets as a segmented control of the workspace (C22 .seg), which scrolls when there are many. */
    const tabs = el('div', 'overflow-x-auto');
    const list = el('div', 'seg');
    list.setAttribute('role', 'group');
    list.setAttribute('aria-label', 'Sheets');
    v.meta.sheets.forEach(s => {
      const b = el('button', '', s.name);
      b.type = 'button';
      b.setAttribute('aria-pressed', String(s.name === v.sheet));
      b.dataset.sheetTab = s.name;
      list.append(b);
    });
    tabs.append(list);
    root.append(tabs);
    const g = v.grid || { rows: [], cols: [], more: 0, from: 0 };
    const bar = el('div', 'flex flex-wrap items-center gap-2 mt-2');
    const shown = g.rows.length ? `Rows ${g.rows[0].n}–${g.rows[g.rows.length - 1].n}${g.more ? `, ${g.more} more` : ''}${g.moreCols ? `; the first ${g.cols.length} columns, ${g.moreCols} more: download it for all` : ''}` : 'An empty sheet';
    bar.append(el('span', 'text-sm text-muted-foreground', shown));
    if (v.from > 0) bar.append(button('Previous rows', 'prev'));
    if (g.more) bar.append(button('Next rows', 'next'));
    const sel = v.sel ? rangeOf(v.sel) : '';
    const embed = button(sel ? 'Embed ' + sel : 'Embed this sheet', 'embed');
    bar.append(embed);
    const head = el('label', 'flex items-center gap-1 text-sm');
    const box = el('input');
    box.type = 'checkbox';
    box.dataset.sheetHead = '1';
    box.checked = !!v.head;
    head.append(box, document.createTextNode('First row is the head'));
    bar.append(head);
    const csv = el('a', 'chip', 'Download CSV');
    csv.href = api(v.id) + '/sheets/csv?' + new URLSearchParams(Object.assign({ sheet: v.sheet }, sel ? { range: sel } : {}));
    csv.dataset.sheetCsv = '1';
    bar.append(csv);
    root.append(bar);
    root.append(gridTable(v, g));
    const said = el('p', 'hint', '');
    said.dataset.sheetSay = '1';
    said.setAttribute('aria-live', 'polite');
    root.append(said);
    const named = [...v.meta.names.map(n => ({ kind: 'name', name: n.name, label: n.name, where: n.sheet + '!' + n.range, value: n.value })),
      ...v.meta.tables.map(t => ({ kind: 'table', name: t.name, label: t.name, where: t.sheet + '!' + t.range }))];
    if (named.length) {
      root.append(el('h3', '', 'Names and tables'));
      const ul = el('ul');
      named.forEach(x => {
        const li = el('li', 'flex flex-wrap items-center gap-2');
        li.append(el('span', '', x.label), el('span', 'text-sm text-muted-foreground', (x.kind === 'table' ? 'Excel table, ' : x.value ? 'value, ' : 'name, ') + x.where));
        const b = button('Embed', 'embed-named');
        b.dataset.kind = x.kind;
        b.dataset.name = x.name;
        li.append(b);
        ul.append(li);
      });
      root.append(ul);
    }
    root.append(el('h3', '', 'Used in pages'));
    if (!v.tables.length) root.append(el('p', 'lab', 'No table of this file has been embedded yet.'));
    else {
      const ul = el('ul');
      v.tables.forEach(t => {
        const li = el('li', 'flex flex-wrap items-center gap-2');
        li.dataset.sheetTable = t.id;
        li.dataset.state = t.state;
        li.append(el('span', '', t.label), el('span', 'text-sm text-muted-foreground', `#${t.id}, ${STATE[t.state] || t.state}${t.moved ? ' (from ' + t.moved + ')' : ''}, used in ${t.uses} place${t.uses === 1 ? '' : 's'}`));
        if (t.problem) li.append(el('span', 'text-sm', t.problem));
        if (t.state === 'changed') {
          const keep = button('Keep it as it is now', 'keep');
          keep.dataset.table = t.id;
          li.append(keep);
        }
        const b = button('Copy the line', 'copy');
        b.dataset.embed = t.embed;
        li.append(b);
        ul.append(li);
      });
      root.append(ul);
    }
  }

  function button(text, act) {
    const b = el('button', 'chip', text);
    b.type = 'button';
    b.dataset.sheetAct = act;
    return b;
  }

  function gridTable(v, g) {
    const wrap = el('div', 'table-container overflow-auto max-h-96 mt-2');
    const table = el('table', 'table');
    table.dataset.variant = 'grid';
    table.dataset.sheetGrid = v.id;
    table.tabIndex = 0;
    table.setAttribute('aria-label', 'Sheet ' + v.sheet + ', mark a range with the mouse or with Shift and the arrows');
    const thead = el('thead', 'sticky top-0 z-10 bg-background');
    const hr = el('tr');
    hr.append(el('th', 'sticky left-0 bg-background', ''));
    g.cols.forEach(c => hr.append(el('th', 'font-mono text-sm', colName(c))));
    thead.append(hr);
    table.append(thead);
    const tbody = el('tbody');
    g.rows.forEach(row => {
      const tr = el('tr');
      const th = el('th', 'sticky left-0 bg-background font-mono text-sm' + (v.mark === row.n ? ' bg-accent' : ''), String(row.n));
      th.dataset.row = row.n;
      tr.append(th);
      row.cells.forEach(c => {
        const td = el('td', (c.k === 'n' || c.k === 'm') ? 'text-right tabular-nums' : '', c.t || '');
        td.dataset.r = row.n;
        td.dataset.c = c.c;
        if (c.rs) td.rowSpan = c.rs;
        if (c.cs) td.colSpan = c.cs;
        if (c.k === 'f') td.title = 'This formula was never calculated: open and save the file in Excel or LibreOffice';
        tr.append(td);
      });
      tbody.append(tr);
    });
    table.append(tbody);
    wrap.append(table);
    if (v.sel) paint(v, table);
    if (v.mark) requestAnimationFrame(() => { const th = table.querySelector(`th[data-row="${v.mark}"]`); if (th) th.scrollIntoView({ block: 'center', behavior: 'instant' }); });
    return wrap;
  }

  const rangeOf = s => {
    const r1 = Math.min(s.r1, s.r2), r2 = Math.max(s.r1, s.r2), c1 = Math.min(s.c1, s.c2), c2 = Math.max(s.c1, s.c2);
    return r1 === r2 && c1 === c2 ? cellName(r1, c1) : cellName(r1, c1) + ':' + cellName(r2, c2);
  };

  function paint(v, table) {
    const s = v.sel;
    const r1 = Math.min(s.r1, s.r2), r2 = Math.max(s.r1, s.r2), c1 = Math.min(s.c1, s.c2), c2 = Math.max(s.c1, s.c2);
    table.querySelectorAll('td[data-r]').forEach(td => {
      const r = +td.dataset.r, c = +td.dataset.c, on = r >= r1 && r <= r2 && c >= c1 && c <= c2;
      td.classList.toggle('bg-accent', on);
      td.setAttribute('aria-selected', String(on));
    });
  }

  /* After the marking changed: the bar names the range, without drawing the table anew. */
  function marked(v) {
    const table = v.root.querySelector('[data-sheet-grid]');
    if (table) paint(v, table);
    const sel = v.sel ? rangeOf(v.sel) : '';
    const embed = v.root.querySelector('[data-sheet-act="embed"]');
    if (embed) embed.textContent = sel ? 'Embed ' + sel : 'Embed this sheet';
    const csv = v.root.querySelector('[data-sheet-csv]');
    if (csv) csv.href = api(v.id) + '/sheets/csv?' + new URLSearchParams(Object.assign({ sheet: v.sheet }, sel ? { range: sel } : {}));
  }

  async function choose(v, body, value) {
    try {
      const t = await json(api(v.id) + '/tables', { method: 'POST', headers: { Accept: 'application/json', 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
      await copy(t.embed);
      await tables(v);
      draw(v);
      say(`Copied ${t.embed}: paste it into a page on a line of its own${value ? ', or inside a sentence with one colon' : ''}.`);
    } catch (e) { say(e.message); }
  }

  async function copy(text) {
    try { await navigator.clipboard.writeText(text); } catch (e) { /* the line shows in the message all the same */ }
  }

  const of = n => { const root = n.closest && n.closest('[data-sheetview]'); return root && VIEWS[root.dataset.sheetview]; };

  document.addEventListener('click', async e => {
    const v = of(e.target);
    if (!v) return;
    const tab = e.target.closest('[data-sheet-tab]');
    if (tab) { v.sheet = tab.dataset.sheetTab; v.mark = 0; await page(v, { from: 0 }).catch(err => say(err.message)); draw(v); return; }
    const td = e.target.closest('td[data-r]');
    if (td) {
      const r = +td.dataset.r, c = +td.dataset.c;
      if (e.shiftKey && v.sel) { v.sel.r2 = r; v.sel.c2 = c; } else v.sel = { r1: r, c1: c, r2: r, c2: c };
      marked(v);
      return;
    }
    const b = e.target.closest('[data-sheet-act]');
    if (!b) return;
    switch (b.dataset.sheetAct) {
      case 'prev': await page(v, { from: Math.max(0, v.from - 500) }).catch(err => say(err.message)); v.mark = 0; draw(v); break;
      case 'next': await page(v, { from: v.from + 500 }).catch(err => say(err.message)); v.mark = 0; draw(v); break;
      case 'embed': choose(v, v.sel ? { kind: 'range', sheet: v.sheet, range: rangeOf(v.sel), head: !!v.head } : { kind: 'sheet', sheet: v.sheet, head: !!v.head }); break;
      case 'embed-named': choose(v, { kind: b.dataset.kind, name: b.dataset.name, head: !!v.head }, b.dataset.kind === 'name'); break;
      case 'copy': await copy(b.dataset.embed); say('Copied ' + b.dataset.embed); break;
      case 'keep':
        try {
          await json(api(v.id) + '/tables/' + encodeURIComponent(b.dataset.table) + '/keep', { method: 'POST', headers: { Accept: 'application/json' } });
          await tables(v);
          draw(v);
          say('Kept: the range shows as it is now, in every page.');
        } catch (e) { say('Not kept: ' + e.message + '. Only who may edit the file keeps a range.'); }
        break;
    }
  });
  document.addEventListener('change', e => {
    const v = of(e.target);
    if (v && e.target.dataset.sheetHead) v.head = e.target.checked;
  });
  /* Dragging over the cells marks a range. */
  let dragging = null;
  document.addEventListener('mousedown', e => {
    const td = e.target.closest && e.target.closest('td[data-r]');
    const v = td && of(td);
    if (!v || e.button !== 0 || e.shiftKey) return;
    dragging = v;
    v.sel = { r1: +td.dataset.r, c1: +td.dataset.c, r2: +td.dataset.r, c2: +td.dataset.c };
    marked(v);
  });
  document.addEventListener('mouseover', e => {
    if (!dragging) return;
    const td = e.target.closest && e.target.closest('td[data-r]');
    if (!td || of(td) !== dragging) return;
    dragging.sel.r2 = +td.dataset.r;
    dragging.sel.c2 = +td.dataset.c;
    marked(dragging);
  });
  document.addEventListener('mouseup', () => { dragging = null; });
  addEventListener('blur', () => { dragging = null; });
  /* The keyboard: the arrows move the marked cell, with Shift they stretch the range. */
  document.addEventListener('keydown', e => {
    const table = e.target.closest && e.target.closest('[data-sheet-grid]');
    const v = table && of(table);
    if (!v || !v.grid || !['ArrowUp', 'ArrowDown', 'ArrowLeft', 'ArrowRight'].includes(e.key)) return;
    e.preventDefault();
    const rows = v.grid.rows.map(r => r.n), cols = v.grid.cols;
    if (!rows.length || !cols.length) return;
    if (!v.sel) v.sel = { r1: rows[0], c1: cols[0], r2: rows[0], c2: cols[0] };
    const step = (list, at, d) => list[Math.min(list.length - 1, Math.max(0, list.indexOf(at) + d))];
    const r = e.shiftKey ? v.sel.r2 : v.sel.r1, c = e.shiftKey ? v.sel.c2 : v.sel.c1;
    const nr = e.key === 'ArrowUp' ? step(rows, r, -1) : e.key === 'ArrowDown' ? step(rows, r, 1) : r;
    const nc = e.key === 'ArrowLeft' ? step(cols, c, -1) : e.key === 'ArrowRight' ? step(cols, c, 1) : c;
    if (e.shiftKey) { v.sel.r2 = nr; v.sel.c2 = nc; } else v.sel = { r1: nr, c1: nc, r2: nr, c2: nc };
    marked(v);
  });

  function mount() {
    document.querySelectorAll('[data-sheets]').forEach(box => {
      const v = view(box.dataset.sheets);
      if (box.firstChild !== v.root) box.replaceChildren(v.root);
    });
  }
  new MutationObserver(() => { if (document.querySelector('[data-sheets]')) mount(); }).observe(document.documentElement, { childList: true, subtree: true });

  window.UDSheets = {
    /* goto opens a file's table view at a sheet and row, as an anchor <sheet>!<range> of a hit of the search names it. */
    goto(id, anchor) {
      const at = String(anchor || '').lastIndexOf('!');
      if (at <= 0) return;
      const sheet = anchor.slice(0, at), m = /^[A-Z]{1,3}(\d+)/.exec(anchor.slice(at + 1));
      pending[id] = { sheet, row: m ? +m[1] : 1 };
      if (VIEWS[id]) { delete VIEWS[id]; }
    },
    /* forget drops the view of a file, so its next opening reads the current version. */
    forget(id) { delete VIEWS[id]; }
  };
})();
