/* SPDX-License-Identifier: MIT */
/* Spreadsheets as tables (ADR-31): the table view of a file. The workspace (demo.js) draws its window as HTML text again
   and again; this script keeps one view per file as DOM and puts it back into the placeholder <div data-sheets="<id>">
   each time, as the PDF preview does. Every text from the file goes in through textContent, never as HTML.

   The view: the visible sheets as C22's tabs, the sheet as C22's grid (.table[data-variant=grid][data-sticky=sheet]), whose
   head of column letters and column of row numbers stay while it scrolls, rows in pages of 500, a range marked with the mouse (drag, Shift+click) or the keyboard (arrows, Shift+arrows),
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
    const v = VIEWS[id] = { id, meta: null, sheet: '', from: 0, grid: null, sel: null, anchor: null, tables: [], root: el('div', 'grid gap-3'), failed: '' };
    v.root.dataset.sheetview = id;
    v.root.append(quiet('Loading the tables…'));
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

  /* The parts of C22 the view is made of: a quiet line, a notice, a small outline button. */
  const quiet = text => el('p', 'text-muted-foreground text-sm', text);
  const notice = text => { const n = el('div', 'alert'); n.append(el('section', '', text)); return n; };

  const STATE = { ok: 'shows as chosen', moved: 'followed to its new place', changed: 'its first row changed: check it', lost: 'no longer to be found' };

  function draw(v) {
    const root = v.root;
    root.replaceChildren();
    if (v.failed) { root.append(notice('This file shows no tables: ' + v.failed)); return; }
    if (!v.meta) { root.append(quiet('Loading the tables…')); return; }
    if (!v.meta.sheets.length) { root.append(notice('This workbook has no visible sheet.')); return; }
    /* The sheets as C22's tabs; the shown sheet is the one panel. The script switches them itself (no Basecoat): a click
       loads the sheet. Every tab stays in the order of the Tab key, as the buttons of before were. */
    const tabs = el('div', 'tabs');
    const list = el('nav');
    list.setAttribute('role', 'tablist');
    list.setAttribute('aria-label', 'Sheets');
    v.meta.sheets.forEach((s, i) => {
      const b = el('button', '', s.name);
      b.type = 'button';
      b.setAttribute('role', 'tab');
      b.id = 'udsheet-' + v.id + '-' + i;
      b.setAttribute('aria-controls', 'udsheet-' + v.id + '-panel');
      b.setAttribute('aria-selected', String(s.name === v.sheet));
      b.dataset.sheetTab = s.name;
      list.append(b);
    });
    const panel = el('div', 'grid gap-2');
    panel.id = 'udsheet-' + v.id + '-panel';
    panel.setAttribute('role', 'tabpanel');
    const on = list.querySelector('[aria-selected="true"]');
    if (on) panel.setAttribute('aria-labelledby', on.id);
    tabs.append(list, panel);
    root.append(tabs);
    const g = v.grid || { rows: [], cols: [], more: 0, from: 0 };
    const bar = el('div', 'flex flex-wrap items-center gap-2');
    const shown = g.rows.length ? `Rows ${g.rows[0].n}–${g.rows[g.rows.length - 1].n}${g.more ? `, ${g.more} more` : ''}${g.moreCols ? `; the first ${g.cols.length} columns, ${g.moreCols} more: download it for all` : ''}` : 'An empty sheet';
    bar.append(el('span', 'text-sm text-muted-foreground', shown));
    if (v.from > 0) bar.append(button('Previous rows', 'prev'));
    if (g.more) bar.append(button('Next rows', 'next'));
    const sel = v.sel ? rangeOf(v.sel) : '';
    const embed = button(sel ? 'Embed ' + sel : 'Embed this sheet', 'embed');
    bar.append(embed);
    const head = el('label', 'label');
    const box = el('input', 'input');
    box.type = 'checkbox';
    box.dataset.sheetHead = '1';
    box.checked = !!v.head;
    head.append(box, document.createTextNode('First row is the head'));
    bar.append(head);
    const csv = el('a', 'btn', 'Download CSV');
    csv.dataset.variant = 'outline';
    csv.dataset.size = 'xs';
    csv.href = api(v.id) + '/sheets/csv?' + new URLSearchParams(Object.assign({ sheet: v.sheet }, sel ? { range: sel } : {}));
    csv.dataset.sheetCsv = '1';
    bar.append(csv);
    panel.append(bar, gridTable(v, g));
    const said = quiet('');
    said.dataset.sheetSay = '1';
    said.setAttribute('aria-live', 'polite');
    panel.append(said);
    const named = [...v.meta.names.map(n => ({ kind: 'name', name: n.name, label: n.name, where: n.sheet + '!' + n.range, value: n.value })),
      ...v.meta.tables.map(t => ({ kind: 'table', name: t.name, label: t.name, where: t.sheet + '!' + t.range }))];
    /* The names and tables of the file, and the tables pages use: C22's entries, the name over a quiet line, the buttons
       at the end. */
    if (named.length) {
      root.append(el('h3', '', 'Names and tables'));
      const list = entries();
      named.forEach(x => {
        const b = button('Embed', 'embed-named');
        b.dataset.kind = x.kind;
        b.dataset.name = x.name;
        list.append(entry(x.label, (x.kind === 'table' ? 'Excel table, ' : x.value ? 'value, ' : 'name, ') + x.where, '', [b]));
      });
      root.append(list);
    }
    root.append(el('h3', '', 'Used in pages'));
    if (!v.tables.length) root.append(quiet('No table of this file has been embedded yet.'));
    else {
      const list = entries();
      v.tables.forEach(t => {
        const acts = [];
        if (t.state === 'changed') {
          const keep = button('Keep it as it is now', 'keep');
          keep.dataset.table = t.id;
          acts.push(keep);
        }
        const b = button('Copy the line', 'copy');
        b.dataset.embed = t.embed;
        acts.push(b);
        const item = entry(t.label, `#${t.id}, ${STATE[t.state] || t.state}${t.moved ? ' (from ' + t.moved + ')' : ''}, used in ${t.uses} place${t.uses === 1 ? '' : 's'}`, t.problem || '', acts);
        item.dataset.sheetTable = t.id;
        item.dataset.state = t.state;
        list.append(item);
      });
      root.append(list);
    }
  }

  function button(text, act) {
    const b = el('button', 'btn', text);
    b.type = 'button';
    b.dataset.variant = 'outline';
    b.dataset.size = 'xs';
    b.dataset.sheetAct = act;
    return b;
  }

  function entries() {
    const n = el('div', 'item-group');
    n.setAttribute('role', 'list');
    return n;
  }

  function entry(name, line, problem, acts) {
    const n = el('article', 'item');
    n.dataset.variant = 'outline';
    n.dataset.size = 'sm';
    n.setAttribute('role', 'listitem');
    const body = el('section');
    body.append(el('h4', '', name), el('p', 'text-muted-foreground text-xs', line));
    if (problem) body.append(el('p', 'text-xs', problem));
    const side = el('aside', 'flex flex-wrap gap-1');
    side.append(...acts);
    n.append(body, side);
    return n;
  }

  /* The sheet is C22's grid: the head and the row numbers stick, the box scrolls both ways within 24rem (C22's
     .table-container around a sticky table), numbers stand at the end (data-type="number"). A marked range is
     aria-selected on its cells and on the letters and numbers of its columns and rows, the cell it started from is
     data-active; the row a hit of the search named is aria-current on its number. */
  function gridTable(v, g) {
    const wrap = el('div', 'table-container');
    const table = el('table', 'table');
    table.dataset.variant = 'grid';
    table.dataset.sticky = 'sheet';
    table.setAttribute('role', 'grid');
    table.setAttribute('aria-multiselectable', 'true');
    table.dataset.sheetGrid = v.id;
    table.tabIndex = 0;
    table.setAttribute('aria-label', 'Sheet ' + v.sheet + ', mark a range with the mouse or with Shift and the arrows');
    const thead = el('thead');
    const hr = el('tr');
    const corner = el('th');
    corner.scope = 'col';
    corner.append(el('span', 'sr-only', 'Row'));
    hr.append(corner);
    g.cols.forEach(c => { const th = el('th', '', colName(c)); th.scope = 'col'; th.dataset.col = c; hr.append(th); });
    thead.append(hr);
    table.append(thead);
    const tbody = el('tbody');
    g.rows.forEach(row => {
      const tr = el('tr');
      const th = el('th', '', String(row.n));
      th.scope = 'row';
      th.dataset.row = row.n;
      /* bridge until C22 has a current row in a grid: the number of the row a hit named is drawn as a selected one. */
      if (v.mark === row.n) { th.setAttribute('aria-current', 'true'); th.setAttribute('aria-selected', 'true'); }
      tr.append(th);
      row.cells.forEach(c => {
        const td = el('td', '', c.t || '');
        /* The kind comes as the number of its letter (a byte in Go's JSON): n a number, m a number as shown, f a formula
           never calculated. */
        const kind = String.fromCharCode(c.k || 0);
        if (kind === 'n' || kind === 'm') td.dataset.type = 'number';
        td.dataset.r = row.n;
        td.dataset.c = c.c;
        if (c.rs) td.rowSpan = c.rs;
        if (c.cs) td.colSpan = c.cs;
        if (kind === 'f') td.title = 'This formula was never calculated: open and save the file in Excel or LibreOffice';
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
      td.setAttribute('aria-selected', String(on));
      td.toggleAttribute('data-active', r === s.r1 && c === s.c1);
    });
    table.querySelectorAll('th[data-row]').forEach(th => th.setAttribute('aria-selected', String(+th.dataset.row >= r1 && +th.dataset.row <= r2)));
    table.querySelectorAll('th[data-col]').forEach(th => th.setAttribute('aria-selected', String(+th.dataset.col >= c1 && +th.dataset.col <= c2)));
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
      /* A page stops early when its text is long (sheet.MaxGridText): the next one starts after the rows it holds. */
      case 'next': await page(v, { from: v.from + ((v.grid && v.grid.rows.length) || 500) }).catch(err => say(err.message)); v.mark = 0; draw(v); break;
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
